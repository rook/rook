package nfs

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"sigs.k8s.io/sig-storage-lib-external-provisioner/v6/mount"
)

type Quotaer interface {
	CreateProjectQuota(projectsFile, directory, limit string) (string, error)
	RemoveProjectQuota(projectID uint16, projectsFile, block string) error
	RestoreProjectQuota() error
}

type Quota struct {
	mutex       *sync.Mutex
	projectsIDs map[string]map[uint16]bool
}

func NewProjectQuota() (Quotaer, error) {
	projectsIDs := map[string]map[uint16]bool{}
	mountEntries, err := findProjectQuotaMount()
	if err != nil {
		return nil, err
	}

	for _, entry := range mountEntries {
		exportName := filepath.Base(entry.Mountpoint)
		projectsIDs[exportName] = map[uint16]bool{}
		projectsFile := filepath.Join(entry.Mountpoint, "projects")
		_, err := os.Stat(projectsFile)
		if os.IsNotExist(err) {
			logger.Infof("creating new project file %s", projectsFile)
			file, cerr := os.Create(projectsFile)
			if cerr != nil {
				return nil, fmt.Errorf("error creating xfs projects file %s: %v", projectsFile, cerr)
			}

			if err := file.Close(); err != nil {
				return nil, err
			}
		} else {
			logger.Infof("found project file %s, restoring project ids", projectsFile)
			re := regexp.MustCompile("(?m:^([0-9]+):/.+$)")
			projectIDs, err := restoreProjectIDs(projectsFile, re)
			if err != nil {
				logger.Errorf("error while populating projectIDs map, there may be errors setting quotas later if projectIDs are reused: %v", err)
			}

			projectsIDs[exportName] = projectIDs
		}
	}

	quota := &Quota{
		mutex:       &sync.Mutex{},
		projectsIDs: projectsIDs,
	}

	if err := quota.RestoreProjectQuota(); err != nil {
		return nil, err
	}

	return quota, nil
}

func findProjectQuotaMount() ([]*mount.Info, error) {
	var entries []*mount.Info
	allEntries, err := mount.GetMounts()
	if err != nil {
		return nil, err
	}

	for _, entry := range allEntries {
		// currently we only support xfs
		if entry.Fstype != "xfs" {
			continue
		}

		if filepath.Dir(entry.Mountpoint) == mountPath && (strings.Contains(entry.VfsOpts, "pquota") || strings.Contains(entry.VfsOpts, "prjquota")) {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

func restoreProjectIDs(projectsFile string, re *regexp.Regexp) (map[uint16]bool, error) {
	ids := map[uint16]bool{}
	digitsRe := "([0-9]+)"
	if !strings.Contains(re.String(), digitsRe) {
		return ids, fmt.Errorf("regexp %s doesn't contain digits submatch %s", re.String(), digitsRe)
	}

	read, err := ioutil.ReadFile(projectsFile) // #nosec
	if err != nil {
		return ids, err
	}

	allMatches := re.FindAllSubmatch(read, -1)
	for _, match := range allMatches {
		digits := match[1]
		if id, err := strconv.ParseUint(string(digits), 10, 16); err == nil {
			ids[uint16(id)] = true
		}
	}

	return ids, nil
}

func (q *Quota) CreateProjectQuota(projectsFile, directory, limit string) (string, error) {
	exportName := filepath.Base(filepath.Dir(projectsFile))

	q.mutex.Lock()
	projectID := uint16(1)
	for ; projectID < math.MaxUint16; projectID++ {
		if _, ok := q.projectsIDs[exportName][projectID]; !ok {
			break
		}
	}

	q.projectsIDs[exportName][projectID] = true
	block := strconv.FormatUint(uint64(projectID), 10) + ":" + directory + ":" + limit + "\n"
	file, err := os.OpenFile(projectsFile, os.O_APPEND|os.O_WRONLY, 0600) // #nosec
	if err != nil {
		q.mutex.Unlock()
		return "", err
	}

	defer func() {
		if err := file.Close(); err != nil {
			logger.Errorf("Error closing file: %s\n", err)
		}
	}()

	if _, err = file.WriteString(block); err != nil {
		q.mutex.Unlock()
		return "", err
	}

	if err := file.Sync(); err != nil {
		q.mutex.Unlock()
		return "", err
	}

	logger.Infof("set project to %s for directory %s with limit %s", projectsFile, directory, limit)
	if err := q.setProject(projectID, projectsFile, directory); err != nil {
		q.mutex.Unlock()
		return "", err
	}

	logger.Infof("set quota for project id %d with limit %s", projectID, limit)
	if err := q.setQuota(projectID, projectsFile, directory, limit); err != nil {
		q.mutex.Unlock()
		_ = q.removeProject(projectID, projectsFile, block)
	}

	q.mutex.Unlock()
	return block, nil
}

func (q *Quota) RemoveProjectQuota(projectID uint16, projectsFile, block string) error {
	return q.removeProject(projectID, projectsFile, block)
}

func (q *Quota) RestoreProjectQuota() error {
	mountEntries, err := findProjectQuotaMount()
	if err != nil {
		return err
	}

	for _, entry := range mountEntries {
		projectsFile := filepath.Join(entry.Mountpoint, "projects")
		if _, err := os.Stat(projectsFile); err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return err
		}
		read, err := ioutil.ReadFile(projectsFile) // #nosec
		if err != nil {
			return err
		}

		re := regexp.MustCompile("(?m:^([0-9]+):(.+):(.+)$\n)")
		matches := re.FindAllSubmatch(read, -1)
		for _, match := range matches {
			projectID, _ := strconv.ParseUint(string(match[1]), 10, 16)
			directory := string(match[2])
			bhard := string(match[3])

			if _, err := os.Stat(directory); os.IsNotExist(err) {
				_ = q.removeProject(uint16(projectID), projectsFile, string(match[0]))
				continue
			}

			if err := q.setProject(uint16(projectID), projectsFile, directory); err != nil {
				return err
			}

			logger.Infof("restoring quotas from project file %s for project id %s", string(match[1]), projectsFile)
			if err := q.setQuota(uint16(projectID), projectsFile, directory, bhard); err != nil {
				return fmt.Errorf("error restoring quota for directory %s: %v", directory, err)
			}
		}
	}

	return nil
}

func (q *Quota) setProject(projectID uint16, projectsFile, directory string) error {
	cmd := exec.Command("xfs_quota", "-x", "-c", fmt.Sprintf("project -s -p %s %s", directory, strconv.FormatUint(uint64(projectID), 10)), filepath.Dir(projectsFile)) // #nosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xfs_quota failed with error: %v, output: %s", err, out)
	}

	return nil
}

func (q *Quota) setQuota(projectID uint16, projectsFile, directory, bhard string) error {
	exportName := filepath.Base(filepath.Dir(projectsFile))
	if !q.projectsIDs[exportName][projectID] {
		return fmt.Errorf("project with id %v has not been added", projectID)
	}

	cmd := exec.Command("xfs_quota", "-x", "-c", fmt.Sprintf("limit -p bhard=%s %s", bhard, strconv.FormatUint(uint64(projectID), 10)), filepath.Dir(projectsFile)) // #nosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xfs_quota failed with error: %v, output: %s", err, out)
	}

	return nil
}

func (q *Quota) removeProject(projectID uint16, projectsFile, block string) error {
	exportName := filepath.Base(filepath.Dir(projectsFile))
	q.mutex.Lock()
	delete(q.projectsIDs[exportName], projectID)
	read, err := ioutil.ReadFile(projectsFile) // #nosec
	if err != nil {
		q.mutex.Unlock()
		return err
	}

	removed := strings.Replace(string(read), block, "", -1)
	err = ioutil.WriteFile(projectsFile, []byte(removed), 0)
	if err != nil {
		q.mutex.Unlock()
		return err
	}

	q.mutex.Unlock()
	return nil
}
