package contracts

type IDockerClient interface {
	Execute(cmd []string) (stdout string, stderr string, err error)
	Stop(cmd []string) (stdout string, stderr string, exitCode int)
	Run(cmd []string) (stdout string, stderr string, err error)
}

