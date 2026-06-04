package model

// TotalCPURequest returns the sum of all container CPU requests in millicores.
func (p PodSpec) TotalCPURequest() int64 {
	var sum int64
	for _, c := range p.Containers {
		sum += c.CPURequest
	}
	return sum
}

// TotalMemRequest returns the sum of all container memory requests in bytes.
func (p PodSpec) TotalMemRequest() int64 {
	var sum int64
	for _, c := range p.Containers {
		sum += c.MemRequest
	}
	return sum
}

// TotalCPULimit returns the sum of all container CPU limits in millicores.
func (p PodSpec) TotalCPULimit() int64 {
	var sum int64
	for _, c := range p.Containers {
		sum += c.CPULimit
	}
	return sum
}

// TotalMemLimit returns the sum of all container memory limits in bytes.
func (p PodSpec) TotalMemLimit() int64 {
	var sum int64
	for _, c := range p.Containers {
		sum += c.MemLimit
	}
	return sum
}

// IsStatic reports whether the replica bounds are fixed (min == max), i.e. there
// is no autoscaling range to span.
func (b ReplicaBounds) IsStatic() bool {
	return b.Min == b.Max
}
