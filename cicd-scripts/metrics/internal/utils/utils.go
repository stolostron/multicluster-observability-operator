// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package utils

// Diff returns the list of added elements (elements from slice b that are not found in a)
// and removed elements (elements from slice a that are not found in b)
func Diff(a, b []string) (added, removed []string) {
	mA := make(map[string]struct{}, len(a))
	for _, x := range a {
		mA[x] = struct{}{}
	}

	mB := make(map[string]struct{}, len(b))
	for _, x := range b {
		mB[x] = struct{}{}
	}

	// Identify elements in b that are not in a
	for x := range mB {
		if _, ok := mA[x]; !ok {
			added = append(added, x)
		}
	}

	// Identify elements in a that are not in b
	for x := range mA {
		if _, ok := mB[x]; !ok {
			removed = append(removed, x)
		}
	}

	return added, removed
}

// Duplicates returns the list of duplicate elements found in the argument slice
func Duplicates(elements []string) []string {
	found := map[string]struct{}{}
	dups := map[string]struct{}{}
	for _, element := range elements {
		if _, ok := found[element]; ok {
			dups[element] = struct{}{}
		} else {
			found[element] = struct{}{}
		}
	}

	ret := make([]string, 0, len(dups))
	for k := range dups {
		ret = append(ret, k)
	}

	return ret
}
