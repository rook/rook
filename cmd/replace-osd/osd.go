package main

type OSD struct {
	ID            int
	DeviceClass   string
	Capacity      float64
	UsedCapacity  float64
	NearfullRatio float64
	Weight        float64
}
