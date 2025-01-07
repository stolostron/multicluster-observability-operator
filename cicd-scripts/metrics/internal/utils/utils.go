package utils

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

	ret := []string{}
	for k := range dups {
		ret = append(ret, k)
	}

	return ret
}
