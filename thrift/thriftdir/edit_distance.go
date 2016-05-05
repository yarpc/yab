package thriftdir

// editDistance returns the distance using Levenshtein distance
func editDistance(s1, s2 string) int {
	prev := make([]int, len(s1)+1)
	cur := make([]int, len(s1)+1)

	for i := range prev {
		prev[i] = i
	}

	for row := 0; row < len(s2); row++ {
		cur[0] = row + 1
		for col := 1; col <= len(s1); col++ {

			var cost int
			if s1[col-1] != s2[row] {
				cost = 1
			}

			cur[col] = minInt(
				cur[col-1]+1,
				prev[col]+1,
				prev[col-1]+cost,
			)
		}

		prev, cur = cur, prev
	}

	return prev[len(prev)-1]
}

func minInt(vals ...int) int {
	min := vals[0]
	for _, v := range vals {
		if v < min {
			min = v
		}
	}
	return min
}
