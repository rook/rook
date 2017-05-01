package contracts


type IDockerClient interface {
	Execute(cmd []string) (stdout string, stderr string, err error)
	Stop(cmd []string) (stdout string, stderr string, err error)
	Run(cmd []string) (stdout string, stderr string, err error)
	ExecuteCmd(cmdArgs []string) (stdout string, stderr string, err error)

}

