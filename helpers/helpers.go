package helpers

import "tree-sit/test/types"

func Subgroup(s string, m []int, idx int) string {
	if idx <= 0 {
		return ""
	}
	i := idx * 2
	if i+1 >= len(m) || m[i] < 0 {
		return ""
	}
	return s[m[i]:m[i+1]]
}

func Containing(fns []types.FunctionDef, line int) string {
	name := ""
	for _, d := range fns {
		if d.StartLine > line {
			break
		}
		name = d.Name
	}
	return name
}
