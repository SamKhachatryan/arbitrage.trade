package main

type Reliability int

const (
	NotReliableAtAll Reliability = iota
	UltraLow
	Low
	Medium
	High
	UltraHigh
)
