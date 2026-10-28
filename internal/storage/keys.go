package storage

type keys []string

func (k keys) scanStart(after string, resuming bool, prefix string) int {
	pos := 0

	if resuming {
		p, found := k.binarySearch(after)
		pos = p

		if found {
			pos++
		}
	}

	if prefix != "" {
		if p, _ := k.binarySearch(prefix); p > pos {
			pos = p
		}
	}

	return pos
}

func (k keys) binarySearch(target string) (int, bool) {
	left, right := 0, len(k)

	for left < right {
		mid := left + (right-left)/2

		if k[mid] < target {
			left = mid + 1
		} else {
			right = mid
		}
	}

	return left, left < len(k) && k[left] == target
}

func (k keys) insert(key string) keys {
	pos, _ := k.binarySearch(key)

	k = append(k, "")
	copy(k[pos+1:], k[pos:])
	k[pos] = key

	return k
}

func (k keys) delete(key string) keys {
	pos, found := k.binarySearch(key)
	if !found {
		return k
	}

	return append(k[:pos], k[pos+1:]...)
}
