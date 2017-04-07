package contracts

type IDockerClient interface {
	Execute(cmd []string) (stdout string, stderr string, exitCode int)
	Stop(cmd []string) (stdout string, stderr string, exitCode int)
	Run(cmd []string) (stdout string, stderr string, exitCode int)
}

