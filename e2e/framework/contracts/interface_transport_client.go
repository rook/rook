package contracts

type ITransportClient interface {
	Execute(cmd []string, optional []string) (stdout string, stderr string, exitCode int)
	Create(cmd []string, optional []string) (stdout string, stderr string, exitCode int)
	Delete(cmd []string, optional []string) (stdout string, stderr string, exitCode int)
	ExecuteCmd(cmd []string) (stdout string, stderr string, err error)
	Apply(cmd []string) (stdout string, stderr string, err error)
	CreateWithStdin(stdinText string) (stdout string, stderr string, exitCode int)
}
