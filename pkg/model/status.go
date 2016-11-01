package model

type HealthStatus int

const (
	HealthOK HealthStatus = iota
	HealthWarning
	HealthError
	HealthUnknown
)

type StatusDetails struct {
	OverallStatus   HealthStatus     `json:"overall"`
	SummaryMessages []StatusSummary  `json:"summary"`
	Monitors        []MonitorSummary `json:"monitors"`
	OSDs            OSDSummary       `json:"osd"`
	PGs             PGSummary        `json:"pg"`
	Usage           UsageSummary     `json:"usage"`
}

type StatusSummary struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message"`
}

type MonitorSummary struct {
	Name     string       `json:"name"`
	Address  string       `json:"address"`
	InQuorum bool         `json:"inQuorum"`
	Status   HealthStatus `json:"status"`
}

type OSDSummary struct {
	Total    int  `json:"total"`
	NumberIn int  `json:"numIn"`
	NumberUp int  `json:"numUp"`
	Full     bool `json:"full"`
	NearFull bool `json:"nearFull"`
}

type PGSummary struct {
	Total       int            `json:"total"`
	StateCounts map[string]int `json:"stateCount"`
}

type UsageSummary struct {
	DataBytes      uint64 `json:"data"`
	UsedBytes      uint64 `json:"used"`
	AvailableBytes uint64 `json:"available"`
	TotalBytes     uint64 `json:"total"`
}

func HealthStatusToString(hs HealthStatus) string {
	switch hs {
	case HealthOK:
		return "OK"
	case HealthWarning:
		return "WARNING"
	case HealthError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
