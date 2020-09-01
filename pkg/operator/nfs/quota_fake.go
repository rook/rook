package nfs

type FakeQuota struct{}

func NewFakeProjectQuota() (Quotaer, error) {
	return &FakeQuota{}, nil
}

func (q *FakeQuota) CreateProjectQuota(projectsFile, directory, limit string) (string, error) {
	return "", nil
}

func (q *FakeQuota) RemoveProjectQuota(projectID uint16, projectsFile, block string) error {
	return nil
}

func (q *FakeQuota) RestoreProjectQuota() error {
	return nil
}
