package objects

type CommandArgs struct {
	Command             string
	SubCommand          string
	CmdArgs             []string
	OptionalArgs        []string
	PipeToStdIn         string
	EnvironmentVariable []string
}

type CommandOut struct {
	StdOut   string
	StdErr   string
	ExitCode int
	Err      error
}
