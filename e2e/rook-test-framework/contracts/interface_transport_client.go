package contracts

type ITransportClient interface {
	Execute(cmd []string) (stdout string, stderr string, exitCode int)
	Create(cmd []string) (stdout string, stderr string, exitCode int)
	Delete(cmd []string) (stdout string, stderr string, exitCode int)
}
