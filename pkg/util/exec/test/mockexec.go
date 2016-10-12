package test

// ******************** MockExecutor ********************
type MockExecutor struct {
	MockExecuteCommand           func(actionName string, command string, arg ...string) error
	MockExecuteCommandPipeline   func(actionName string, command string) (string, error)
	MockExecuteCommandWithOutput func(actionName string, command string, arg ...string) (string, error)
}

func (e *MockExecutor) ExecuteCommand(actionName string, command string, arg ...string) error {
	if e.MockExecuteCommand != nil {
		return e.MockExecuteCommand(actionName, command, arg...)
	}

	return nil
}

func (e *MockExecutor) ExecuteCommandPipeline(actionName string, command string) (string, error) {
	if e.MockExecuteCommandPipeline != nil {
		return e.MockExecuteCommandPipeline(actionName, command)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithOutput(actionName string, command string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutput != nil {
		return e.MockExecuteCommandWithOutput(actionName, command, arg...)
	}

	return "", nil
}
