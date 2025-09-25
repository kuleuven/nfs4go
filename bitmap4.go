package nfs4go

import (
	"slices"
	"strings"
)

func bitmap4Encode(x map[int]bool) []uint32 {
	maxInt := 0

	for v := range x {
		if v > maxInt {
			maxInt = v
		}
	}

	size := maxInt / 32

	if maxInt%32 > 0 {
		size += 1
	}

	rs := make([]uint32, size)

	for v, on := range x {
		if !on {
			continue
		}

		i := v / 32
		j := v % 32
		s := uint32(1) << j
		rs[i] |= s
	}

	return rs
}

func bitmap4Decode(nums []uint32) map[int]bool {
	x := map[int]bool{}

	if nums == nil {
		return x
	}

	for i, v := range nums {
		for j := 31; j >= 0; j-- {
			s := uint32(1) << j
			n := 32*i + j
			x[n] = s&v == s
		}
	}

	return x
}

func bitmapString(x map[int]bool) string {
	attrs := []string{}

	for v, on := range x {
		if !on {
			continue
		}

		name, _ := GetAttrNameByID(v)

		attrs = append(attrs, name)
	}

	slices.Sort(attrs)

	return strings.Join(attrs, ",")
}
