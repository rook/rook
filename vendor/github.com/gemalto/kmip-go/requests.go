package kmip

// This is a WIP implementation of a KMIP server.  The code is mostly based on the http server in
// the golang standard library.  It is functional, but not all of the features of the http server
// have been ported over yet, and some of the stuff in here still refers to http stuff.
//
// The responsibility of handling a request is broken up into 3 layers of handlers: ProtocolHandler, MessageHandler,
// and ItemHandler.  Each of these handlers delegates details to the next layer.  Using the http
// package as an analogy, ProtocolHandler is similar to the wire-level HTTP protocol handling in
// http.Server and http.Transport.  MessageHandler parses KMIP TTLV bytes into golang request and response structs.
// ItemHandler is a bit like http.ServeMux, routing particular KMIP operations to registered handlers.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ansel1/merry"
	"github.com/gemalto/flume"
	"github.com/gemalto/kmip-go/kmip14"
	"github.com/gemalto/kmip-go/ttlv"
	"github.com/google/uuid"
)

var serverLog = flume.New("kmip_server")

// Server serves KMIP protocol connections from a net.Listener.  Because KMIP is a connection-oriented
// protocol, unlike HTTP, each connection ends up being serviced by a dedicated goroutine (rather than
// each request).  For each KMIP connection, requests are processed serially.  The handling
// of the request is delegated to the ProtocolHandler.
//
// Limitations:
//
// This implementation is functional (it can respond to KMIP requests), but incomplete.  Some of the
// connection management features of the http package haven't been ported over, and also, there is
// currently no connection-context in which to store things like an authentication or session management.
// Since HTTP is an intrinsically stateless model, it makes sense for the http package to delegate session
// management to third party packages, but for KMIP, it would makes sense for there to be some first
// class support for a connection context.
//
// This package also only handles the binary TTLV encoding for now.  It may make sense for this
// server to detect or support the XML and JSON encodings as well.  It may also makes sense to support
// KMIP requests over HTTP, perhaps by adapting ProtocolHandler to an http.Handler or something.
type Server struct {
	Handler ProtocolHandler

	mu         sync.Mutex
	listeners  map[*net.Listener]struct{}
	inShutdown int32 // accessed atomically (non-zero means we're in Shutdown)
}

// ErrServerClosed is returned by the Server's Serve, ServeTLS, ListenAndServe,
// and ListenAndServeTLS methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("http: Server closed")

// Serve accepts incoming connections on the Listener l, creating a
// new service goroutine for each. The service goroutines read requests and
// then call srv.MessageHandler to reply to them.
//
// Serve always returns a non-nil error and closes l.
// After Shutdown or Close, the returned error is ErrServerClosed.
func (srv *Server) Serve(l net.Listener) error {
	// if fn := testHookServerServe; fn != nil {
	// 	fn(srv, l) // call hook with unwrapped listener
	// }

	l = &onceCloseListener{Listener: l}
	defer l.Close()

	if !srv.trackListener(&l, true) {
		return ErrServerClosed
	}
	defer srv.trackListener(&l, false)

	var tempDelay time.Duration     // how long to sleep on accept failure
	baseCtx := context.Background() // base is always background, per Issue 16220
	ctx := baseCtx
	// ctx := context.WithValue(baseCtx, ServerContextKey, srv)
	for {
		rw, e := l.Accept()
		if e != nil {
			if srv.shuttingDown() {
				return ErrServerClosed
			}
			var ne net.Error
			if errors.As(e, &ne) && ne.Timeout() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}

				if maxDelay := 1 * time.Second; tempDelay > maxDelay {
					tempDelay = maxDelay
				}
				// srv.logf("http: Accept error: %v; retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return e
		}
		tempDelay = 0
		c := &conn{server: srv, rwc: rw}
		// c.setState(c.rwc, StateNew) // before Serve can return
		go c.serve(ctx)
	}
}

// Close immediately closes all active net.Listeners and any
// connections in state StateNew, StateActive, or StateIdle. For a
// graceful shutdown, use Shutdown.
//
// Close does not attempt to close (and does not even know about)
// any hijacked connections, such as WebSockets.
//
// Close returns any error returned from closing the Server's
// underlying Listener(s).
func (srv *Server) Close() error {
	atomic.StoreInt32(&srv.inShutdown, 1)
	srv.mu.Lock()
	defer srv.mu.Unlock()
	// srv.closeDoneChanLocked()
	err := srv.closeListenersLocked()
	// for c := range srv.activeConn {
	// 	c.rwc.Close()
	// 	delete(srv.activeConn, c)
	// }
	return err
}

// shutdownPollInterval is how often we poll for quiescence
// during Server.Shutdown. This is lower during tests, to
// speed up tests.
// Ideally we could find a solution that doesn't involve polling,
// but which also doesn't have a high runtime cost (and doesn't
// involve any contentious mutexes), but that is left as an
// exercise for the reader.
var shutdownPollInterval = 500 * time.Millisecond

// Shutdown gracefully shuts down the server without interrupting any
// active connections. Shutdown works by first closing all open
// listeners, then closing all idle connections, and then waiting
// indefinitely for connections to return to idle and then shut down.
// If the provided context expires before the shutdown is complete,
// Shutdown returns the context's error, otherwise it returns any
// error returned from closing the Server's underlying Listener(s).
//
// When Shutdown is called, Serve, ListenAndServe, and
// ListenAndServeTLS immediately return ErrServerClosed. Make sure the
// program doesn't exit and waits instead for Shutdown to return.
//
// Shutdown does not attempt to close nor wait for hijacked
// connections such as WebSockets. The caller of Shutdown should
// separately notify such long-lived connections of shutdown and wait
// for them to close, if desired. See RegisterOnShutdown for a way to
// register shutdown notification functions.
//
// Once Shutdown has been called on a server, it may not be reused;
// future calls to methods such as Serve will return ErrServerClosed.
func (srv *Server) Shutdown(_ context.Context) error {
	atomic.StoreInt32(&srv.inShutdown, 1)

	srv.mu.Lock()
	lnerr := srv.closeListenersLocked()
	// srv.closeDoneChanLocked()
	// for _, f := range srv.onShutdown {
	// 	go f()
	// }
	srv.mu.Unlock()

	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()
	return lnerr
	// for {
	// 	if srv.closeIdleConns() {
	// 		return lnerr
	// 	}
	// 	select {
	// 	case <-ctx.Done():
	// 		return ctx.Err()
	// 	case <-ticker.C:
	// 	}
	// }
}

func (srv *Server) closeListenersLocked() error {
	var err error
	for ln := range srv.listeners {
		if cerr := (*ln).Close(); cerr != nil && err == nil {
			err = cerr
		}
		delete(srv.listeners, ln)
	}
	return err
}

// trackListener adds or removes a net.Listener to the set of tracked
// listeners.
//
// We store a pointer to interface in the map set, in case the
// net.Listener is not comparable. This is safe because we only call
// trackListener via Serve and can track+defer untrack the same
// pointer to local variable there. We never need to compare a
// Listener from another caller.
//
// It reports whether the server is still up (not Shutdown or Closed).
func (srv *Server) trackListener(ln *net.Listener, add bool) bool {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.listeners == nil {
		srv.listeners = make(map[*net.Listener]struct{})
	}
	if add {
		if srv.shuttingDown() {
			return false
		}
		srv.listeners[ln] = struct{}{}
	} else {
		delete(srv.listeners, ln)
	}
	return true
}

func (srv *Server) shuttingDown() bool {
	return atomic.LoadInt32(&srv.inShutdown) != 0
}

type conn struct {
	rwc        net.Conn
	remoteAddr string
	localAddr  string
	tlsState   *tls.ConnectionState
	// cancelCtx cancels the connection-level context.
	cancelCtx context.CancelFunc

	// bufr reads from rwc.
	bufr *bufio.Reader
	dec  *ttlv.Decoder

	server *Server
}

func (c *conn) close() {
	// TODO: http package has a buffered writer on the conn too, which is flushed here
	_ = c.rwc.Close()
}

// Serve a new connection.
func (c *conn) serve(ctx context.Context) {
	ctx = flume.WithLogger(ctx, serverLog)
	ctx, cancelCtx := context.WithCancel(ctx)
	c.cancelCtx = cancelCtx
	c.remoteAddr = c.rwc.RemoteAddr().String()
	c.localAddr = c.rwc.LocalAddr().String()
	// ctx = context.WithValue(ctx, LocalAddrContextKey, c.rwc.LocalAddr())
	defer func() {
		if err := recover(); err != nil {
			// if err := recover(); err != nil && err != ErrAbortHandler {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			serverLog.Error("kmip: panic in serve", "remoteAddr", c.remoteAddr, "error", err, "stack", buf)
		}
		cancelCtx()
		// if !c.hijacked() {
		c.close()
		//	c.setState(c.rwc, StateClosed)
		//}
	}()

	if tlsConn, ok := c.rwc.(*tls.Conn); ok {
		// if d := c.server.ReadTimeout; d != 0 {
		// 	c.rwc.SetReadDeadline(time.Now().Add(d))
		// }
		// if d := c.server.WriteTimeout; d != 0 {
		// 	c.rwc.SetWriteDeadline(time.Now().Add(d))
		// }
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			serverLog.Error("kmip: TLS handshake error", "remoteAddr", c.rwc.RemoteAddr(), "error", err)
			return
		}
		c.tlsState = new(tls.ConnectionState)
		*c.tlsState = tlsConn.ConnectionState()
		// if proto := c.tlsState.NegotiatedProtocol; validNPN(proto) {
		// 	if fn := c.server.TLSNextProto[proto]; fn != nil {
		// 		h := initNPNRequest{tlsConn, serverHandler{c.server}}
		// 		fn(c.server, tlsConn, h)
		// 	}
		// 	return
		// }
	}

	// TODO: do we really need instance pooling here?  We expect KMIP connections to be long lasting
	c.dec = ttlv.NewDecoder(c.rwc)
	c.bufr = bufio.NewReader(c.rwc)
	// c.bufw = newBufioWriterSize(checkConnErrorWriter{c}, 4<<10)

	for {
		w, err := c.readRequest(ctx)
		// if c.r.remain != c.server.initialReadLimitSize() {
		//  If we read any bytes off the wire, we're active.
		// c.setState(c.rwc, StateActive)
		// }
		if err != nil {
			if merry.Is(err, io.EOF) {
				serverLog.Info("kmip: client closed connection", "remoteAddr", c.remoteAddr)
				return
			}

			// TODO: do something with this error
			panic(err)
			// const errorHeaders= "\r\nContent-Type: text/plain; charset=utf-8\r\nConnection: close\r\n\r\n"
			//
			// if err == errTooLarge {
			// 	// Their HTTP client may or may not be
			// 	// able to read this if we're
			// 	// responding to them and hanging up
			// 	// while they're still writing their
			// 	// request. Undefined behavior.
			// 	const publicErr= "431 Request Header Fields Too Large"
			// 	fmt.Fprintf(c.rwc, "HTTP/1.1 "+publicErr+errorHeaders+publicErr)
			// 	c.closeWriteAndWait()
			// 	return
			// }
			// if isCommonNetReadError(err) {
			// 	return // don't reply
			// }
			//
			// publicErr := "400 Bad Request"
			// if v, ok := err.(badRequestError); ok {
			// 	publicErr = publicErr + ": " + string(v)
			// }
			//
			// fmt.Fprintf(c.rwc, "HTTP/1.1 "+publicErr+errorHeaders+publicErr)
			// return
		}

		// Expect 100 Continue support
		// req := w.req
		// if req.expectsContinue() {
		// 	if req.ProtoAtLeast(1, 1) && req.ContentLength != 0 {
		// 		// Wrap the Body reader with one that replies on the connection
		// 		req.Body = &expectContinueReader{readCloser: req.Body, resp: w}
		// 	}
		// } else if req.Header.get("Expect") != "" {
		// 	w.sendExpectationFailed()
		// 	return
		// }

		// c.curReq.Store(w)

		// if requestBodyRemains(req.Body) {
		// 	registerOnHitEOF(req.Body, w.conn.r.startBackgroundRead)
		// } else {
		// 	w.conn.r.startBackgroundRead()
		// }

		// HTTP cannot have multiple simultaneous active requests.[*]
		// Until the server replies to this request, it can't read another,
		// so we might as well run the handler in this goroutine.
		// [*] Not strictly true: HTTP pipelining. We could let them all process
		// in parallel even if their responses need to be serialized.
		// But we're not going to implement HTTP pipelining because it
		// was never deployed in the wild and the answer is HTTP/2.

		h := c.server.Handler
		if h == nil {
			h = DefaultProtocolHandler
		}

		// var resp ResponseMessage
		// err = c.server.MessageHandler.Handle(ctx, w, &resp)
		// TODO: this cancelCtx() was created at the connection level, not the request level.  Need to
		// figure out how to handle connection vs request timeouts and cancels.
		// cancelCtx()

		// TODO: use recycled buffered writer
		writer := bufio.NewWriter(c.rwc)
		h.ServeKMIP(ctx, w, writer)
		err = writer.Flush()
		if err != nil {
			// TODO: handle error
			panic(err)
		}

		// serverHandler{c.server}.ServeHTTP(w, w.req)
		// w.cancelCtx()
		// if c.hijacked() {
		// 	return
		// }
		// w.finishRequest()
		// if !w.shouldReuseConnection() {
		// 	if w.requestBodyLimitHit || w.closedRequestBodyEarly() {
		// 		c.closeWriteAndWait()
		// 	}
		// 	return
		// }
		// c.setState(c.rwc, StateIdle)
		// c.curReq.Store((*response)(nil))

		// if !w.conn.server.doKeepAlives() {
		// 	// We're in shutdown mode. We might've replied
		// 	// to the user without "Connection: close" and
		// 	// they might think they can send another
		// 	// request, but such is life with HTTP/1.1.
		// 	return
		// }
		//
		// if d := c.server.idleTimeout(); d != 0 {
		// 	c.rwc.SetReadDeadline(time.Now().Add(d))
		// 	if _, err := c.bufr.Peek(4); err != nil {
		// 		return
		// 	}
		// }
		// c.rwc.SetReadDeadline(time.Time{})
	}
}

// Read next request from connection.
func (c *conn) readRequest(_ context.Context) (w *Request, err error) {
	// if c.hijacked() {
	// 	return nil, ErrHijacked
	// }

	// var (
	// 	wholeReqDeadline time.Time // or zero if none
	// 	hdrDeadline      time.Time // or zero if none
	// )
	// t0 := time.Now()
	// if d := c.server.readHeaderTimeout(); d != 0 {
	// 	hdrDeadline = t0.Add(d)
	// }
	// if d := c.server.ReadTimeout; d != 0 {
	// 	wholeReqDeadline = t0.Add(d)
	// }
	// c.rwc.SetReadDeadline(hdrDeadline)
	// if d := c.server.WriteTimeout; d != 0 {
	// 	defer func() {
	// 		c.rwc.SetWriteDeadline(time.Now().Add(d))
	// 	}()
	// }

	// c.r.setReadLimit(c.server.initialReadLimitSize())
	// if c.lastMethod == "POST" {
	//  RFC 7230 section 3 tolerance for old buggy clients.
	// peek, _ := c.bufr.Peek(4) // ReadRequest will get err below
	// c.bufr.Discard(numLeadingCRorLF(peek))
	// }
	ttlvVal, err := c.dec.NextTTLV()
	if err != nil {
		return nil, err
	}
	// if err != nil {
	// if c.r.hitReadLimit() {
	// 	return nil, errTooLarge
	// }
	// }

	// TODO: use pooling to recycle requests?
	req := &Request{
		TTLV:       ttlvVal,
		RemoteAddr: c.remoteAddr,
		LocalAddr:  c.localAddr,
		TLS:        c.tlsState,
	}

	// c.r.setInfiniteReadLimit()

	// Adjust the read deadline if necessary.
	// if !hdrDeadline.Equal(wholeReqDeadline) {
	// 	c.rwc.SetReadDeadline(wholeReqDeadline)
	// }

	return req, nil
}

// Request represents a KMIP request.
type Request struct {
	// TTLV will hold the entire body of the request.
	TTLV                ttlv.TTLV
	Message             *RequestMessage
	CurrentItem         *RequestBatchItem
	DisallowExtraValues bool

	// TLS holds the TLS state of the connection this request was received on.
	TLS        *tls.ConnectionState
	RemoteAddr string
	LocalAddr  string

	IDPlaceholder string

	decoder *ttlv.Decoder
}

// coerceToTTLV attempts to coerce an interface value to TTLV.
// In most production scenarios, this is intended to be used in
// places where the value is already a TTLV, and just needs to be
// type cast.  If v is not TTLV, it will be marshaled.  This latter
// behavior is slow, so it should be used only in tests.
func coerceToTTLV(v interface{}) (ttlv.TTLV, error) {
	switch t := v.(type) {
	case nil:
		return nil, nil
	case ttlv.TTLV:
		return t, nil
	default:
		return ttlv.Marshal(v)
	}
}

// Unmarshal unmarshals ttlv into structures.  Handlers should prefer this
// method over than their own Decoders or Unmarshal().  This method
// enforces rules about whether extra fields are allowed, and reuses
// buffers for efficiency.
func (r *Request) Unmarshal(ttlv ttlv.TTLV, into interface{}) error {
	if len(ttlv) == 0 {
		return nil
	}
	r.decoder.Reset(bytes.NewReader(ttlv))
	return r.decoder.Decode(into)
}

func (r *Request) DecodePayload(v interface{}) error {
	if r.CurrentItem == nil {
		return nil
	}
	ttlvVal, err := coerceToTTLV(r.CurrentItem.RequestPayload)
	if err != nil {
		return err
	}
	return r.Unmarshal(ttlvVal, v)
}

// onceCloseListener wraps a net.Listener, protecting it from
// multiple Close calls.
type onceCloseListener struct {
	net.Listener
	once     sync.Once
	closeErr error
}

func (oc *onceCloseListener) Close() error {
	oc.once.Do(oc.close)
	return oc.closeErr
}

func (oc *onceCloseListener) close() { oc.closeErr = oc.Listener.Close() }

type ResponseWriter interface {
	io.Writer
}

// ProtocolHandler is responsible for handling raw requests read off the wire.  The
// *Request object will only have TTLV field populated.  The response should
// be written directly to the ResponseWriter.
//
// The default implemention of ProtocolHandler is StandardProtocolHandler.
type ProtocolHandler interface {
	ServeKMIP(ctx context.Context, req *Request, resp ResponseWriter)
}

// MessageHandler handles KMIP requests which have already be decoded.  The *Request
// object's Message field will be populated from the decoded TTLV.  The *Response
// object will always be non-nil, and its ResponseHeader will be populated.  The
// MessageHandler usually shouldn't modify the ResponseHeader: the ProtocolHandler
// is responsible for the header.  The MessageHandler just needs to populate
// the response batch items.
//
// The default implementation of MessageHandler is OperationMux.
type MessageHandler interface {
	HandleMessage(ctx context.Context, req *Request, resp *Response)
}

// ItemHandler handles a single batch item in a KMIP request.  The *Request
// object's CurrentItem field will be populated with item to be handled.
type ItemHandler interface {
	HandleItem(ctx context.Context, req *Request) (item *ResponseBatchItem, err error)
}

type ProtocolHandlerFunc func(context.Context, *Request, ResponseWriter)

func (f ProtocolHandlerFunc) ServeKMIP(ctx context.Context, r *Request, w ResponseWriter) {
	f(ctx, r, w)
}

type MessageHandlerFunc func(context.Context, *Request, *Response)

func (f MessageHandlerFunc) HandleMessage(ctx context.Context, req *Request, resp *Response) {
	f(ctx, req, resp)
}

type ItemHandlerFunc func(context.Context, *Request) (*ResponseBatchItem, error)

func (f ItemHandlerFunc) HandleItem(ctx context.Context, req *Request) (item *ResponseBatchItem, err error) {
	return f(ctx, req)
}

var DefaultProtocolHandler = &StandardProtocolHandler{
	MessageHandler: DefaultOperationMux,
	ProtocolVersion: ProtocolVersion{
		ProtocolVersionMajor: 1,
		ProtocolVersionMinor: 4,
	},
}

var DefaultOperationMux = &OperationMux{}

// StandardProtocolHandler is the default ProtocolHandler implementation.  It
// handles decoding the request and encoding the response, as well as protocol
// level tasks like version negotiation and correlation values.
//
// It delegates handling of the request to a MessageHandler.
type StandardProtocolHandler struct {
	ProtocolVersion ProtocolVersion
	MessageHandler  MessageHandler

	LogTraffic bool
}

func (h *StandardProtocolHandler) parseMessage(_ context.Context, req *Request) error {
	ttlvV := req.TTLV
	if err := ttlvV.Valid(); err != nil {
		return merry.Prepend(err, "invalid ttlv")
	}

	if ttlvV.Tag() != kmip14.TagRequestMessage {
		return merry.Errorf("invalid tag: expected RequestMessage, was %s", ttlvV.Tag().String())
	}

	var message RequestMessage
	err := ttlv.Unmarshal(ttlvV, &message)
	if err != nil {
		return merry.Prepend(err, "failed to parse message")
	}

	req.Message = &message

	return nil
}

var responsePool = sync.Pool{}

type Response struct {
	ResponseMessage
	buf bytes.Buffer
	enc *ttlv.Encoder
}

func newResponse() *Response {
	v := responsePool.Get()
	if v != nil {
		r, _ := v.(*Response)
		r.reset()
		return r
	}
	r := Response{}
	r.enc = ttlv.NewEncoder(&r.buf)
	return &r
}

func releaseResponse(r *Response) {
	responsePool.Put(r)
}

func (r *Response) reset() {
	if r == nil {
		return
	}
	r.BatchItem = nil
	r.ResponseMessage = ResponseMessage{}
	r.buf.Reset()
}

func (r *Response) Bytes() []byte {
	r.buf.Reset()
	err := r.enc.Encode(&r.ResponseMessage)
	if err != nil {
		panic(err)
	}

	return r.buf.Bytes()
}

func (r *Response) errorResponse(reason kmip14.ResultReason, msg string) {
	r.BatchItem = []ResponseBatchItem{
		{
			ResultStatus:  kmip14.ResultStatusOperationFailed,
			ResultReason:  reason,
			ResultMessage: msg,
		},
	}
}

func (h *StandardProtocolHandler) handleRequest(ctx context.Context, req *Request, resp *Response) (logger flume.Logger) {
	// create a server correlation value, which is like a unique transaction ID
	scv := uuid.New().String()

	// create a logger for the transaction, seeded with the scv
	logger = flume.FromContext(ctx).With("scv", scv)
	// attach the logger to the context, so it is available to the handling chain
	ctx = flume.WithLogger(ctx, logger)

	// TODO: it's unclear how the full protocol negogiation is supposed to work
	// should server be pinned to a particular version?  Or should we try and negogiate a common version?
	resp.ResponseHeader.ProtocolVersion = h.ProtocolVersion
	resp.ResponseHeader.TimeStamp = time.Now()
	resp.ResponseHeader.BatchCount = len(resp.BatchItem)
	resp.ResponseHeader.ServerCorrelationValue = scv

	if err := h.parseMessage(ctx, req); err != nil {
		resp.errorResponse(kmip14.ResultReasonInvalidMessage, err.Error())
		return logger
	}

	ccv := req.Message.RequestHeader.ClientCorrelationValue
	// add the client correlation value to the logging context.  This value uniquely
	// identifies the client, and is supposed to be included in server logs
	logger = logger.With("ccv", ccv)
	ctx = flume.WithLogger(ctx, logger)
	resp.ResponseHeader.ClientCorrelationValue = req.Message.RequestHeader.ClientCorrelationValue

	clientMajorVersion := req.Message.RequestHeader.ProtocolVersion.ProtocolVersionMajor
	if clientMajorVersion != h.ProtocolVersion.ProtocolVersionMajor {
		resp.errorResponse(kmip14.ResultReasonInvalidMessage,
			fmt.Sprintf("mismatched protocol versions, client: %d, server: %d", clientMajorVersion, h.ProtocolVersion.ProtocolVersionMajor))
		return logger
	}

	// set a flag hinting to handlers that extra fields should not be tolerated when
	// unmarshaling payloads.  According to spec, if server and client protocol version
	// minor versions match, then extra fields should cause an error.  Not sure how to enforce
	// this in this higher level handler, since we (the protocol/message handlers) don't unmarshal the payload.
	// That's done by a particular item handler.
	req.DisallowExtraValues = req.Message.RequestHeader.ProtocolVersion.ProtocolVersionMinor == h.ProtocolVersion.ProtocolVersionMinor
	req.decoder = ttlv.NewDecoder(nil)
	req.decoder.DisallowExtraValues = req.DisallowExtraValues

	h.MessageHandler.HandleMessage(ctx, req, resp)
	resp.ResponseHeader.BatchCount = len(resp.BatchItem)

	respTTLV := resp.Bytes()

	if req.Message.RequestHeader.MaximumResponseSize > 0 && len(respTTLV) > req.Message.RequestHeader.MaximumResponseSize {
		// new error resp
		resp.errorResponse(kmip14.ResultReasonResponseTooLarge, "")
	}

	return logger
}

func (h *StandardProtocolHandler) ServeKMIP(ctx context.Context, req *Request, writer ResponseWriter) {
	// we precreate the response object and pass it down to handlers, because due
	// the guidance in the spec on the Maximum Response Size, it will be necessary
	// for handlers to recalculate the response size after each batch item, which
	// requires re-encoding the entire response. Seems inefficient.
	resp := newResponse()
	logger := h.handleRequest(ctx, req, resp)

	var err error
	if h.LogTraffic {
		ttlvV := resp.Bytes()

		logger.Debug("traffic log", "request", req.TTLV.String(), "response", ttlv.TTLV(ttlvV).String())
		_, err = writer.Write(ttlvV)
	} else {
		_, err = resp.buf.WriteTo(writer)
	}
	if err != nil {
		panic(err)
	}

	releaseResponse(resp)
}

// func (r *ResponseMessage) addFailure(reason kmip14.ResultReason, msg string) {
// 	if msg == "" {
// 		msg = reason.String()
// 	}
// 	r.BatchItem = append(r.BatchItem, ResponseBatchItem{
// 		ResultStatus:  kmip14.ResultStatusOperationFailed,
// 		ResultReason:  reason,
// 		ResultMessage: msg,
// 	})
// }

// OperationMux is an implementation of MessageHandler which handles each batch item in the request
// by routing the operation to an ItemHandler.  The ItemHandler performs the operation, and returns
// either a *ResponseBatchItem, or an error.  If it returns an error, the error is passed to
// ErrorHandler, which converts it into a error *ResponseBatchItem.  OperationMux handles correlating
// items in the request to items in the response.
type OperationMux struct {
	mu       sync.RWMutex
	handlers map[kmip14.Operation]ItemHandler
	// ErrorHandler defaults to the DefaultErrorHandler.
	ErrorHandler ErrorHandler
}

// ErrorHandler converts a golang error into a *ResponseBatchItem (which should hold information
// about the error to convey back to the client).
type ErrorHandler interface {
	HandleError(err error) *ResponseBatchItem
}

type ErrorHandlerFunc func(err error) *ResponseBatchItem

func (f ErrorHandlerFunc) HandleError(err error) *ResponseBatchItem {
	return f(err)
}

// DefaultErrorHandler tries to map errors to ResultReasons.
var DefaultErrorHandler = ErrorHandlerFunc(func(err error) *ResponseBatchItem {
	reason := GetResultReason(err)
	if reason == kmip14.ResultReason(0) {
		// error not handled
		return nil
	}

	// prefer user message, but fall back on message
	msg := merry.UserMessage(err)
	if msg == "" {
		msg = merry.Message(err)
	}
	return newFailedResponseBatchItem(reason, msg)
})

func newFailedResponseBatchItem(reason kmip14.ResultReason, msg string) *ResponseBatchItem {
	return &ResponseBatchItem{
		ResultStatus:  kmip14.ResultStatusOperationFailed,
		ResultReason:  reason,
		ResultMessage: msg,
	}
}

func (m *OperationMux) bi(ctx context.Context, req *Request, reqItem *RequestBatchItem) *ResponseBatchItem {
	req.CurrentItem = reqItem
	h := m.handlerForOp(reqItem.Operation)
	if h == nil {
		return newFailedResponseBatchItem(kmip14.ResultReasonOperationNotSupported, "")
	}

	resp, err := h.HandleItem(ctx, req)
	if err != nil {
		eh := m.ErrorHandler
		if eh == nil {
			eh = DefaultErrorHandler
		}
		resp = eh.HandleError(err)
		if resp == nil {
			// errors which don't convert just panic
			panic(err)
		}
	}

	return resp
}

func (m *OperationMux) HandleMessage(ctx context.Context, req *Request, resp *Response) {
	for i := range req.Message.BatchItem {
		reqItem := &req.Message.BatchItem[i]
		respItem := m.bi(ctx, req, reqItem)
		respItem.Operation = reqItem.Operation
		respItem.UniqueBatchItemID = reqItem.UniqueBatchItemID
		resp.BatchItem = append(resp.BatchItem, *respItem)
	}
}

func (m *OperationMux) Handle(op kmip14.Operation, handler ItemHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.handlers == nil {
		m.handlers = map[kmip14.Operation]ItemHandler{}
	}

	m.handlers[op] = handler
}

func (m *OperationMux) handlerForOp(op kmip14.Operation) ItemHandler {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.handlers[op]
}

// func (m *OperationMux) missingHandler(_ context.Context, _ *Request, resp *ResponseMessage) error {
// 	resp.addFailure(kmip14.ResultReasonOperationNotSupported, "")
// 	return nil
// }
