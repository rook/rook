package enums

import (
	"errors"
	"strings"
)

type RookPlatformType int

const (
	Kubernetes RookPlatformType = iota + 1
	BareMetal
	StandAlone
	None
)

var platforms = [...]string{
	"Kubernetes",
	"BareMetal",
	"StandAlone",
	"None",
}

func (platform RookPlatformType) String() string {
	return platforms[platform-1]
}

func GetRookPlatFormTypeFromString(name string) (RookPlatformType, error) {
	switch {
	case strings.EqualFold(name, Kubernetes.String()):
		return Kubernetes, nil
	case strings.EqualFold(name, BareMetal.String()):
		return BareMetal, nil
	case strings.EqualFold(name, StandAlone.String()):
		return StandAlone, nil
	case strings.EqualFold(name, None.String()):
		return None, nil
	default:
		return None, errors.New("Unsupported Rook Platform Type: " + name)
	}
}
