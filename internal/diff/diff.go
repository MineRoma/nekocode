package diff

import "strings"

type op struct {
	kind byte
	line string
}

func Unified(path, before, after string) string {
	if before == after {
		return "(no changes)"
	}
	a := split(before)
	b := split(after)
	ops := lcs(a, b)
	var out strings.Builder
	out.WriteString("--- a/" + path + "\n")
	out.WriteString("+++ b/" + path + "\n")
	out.WriteString("@@ full file @@\n")
	for _, item := range ops {
		out.WriteByte(item.kind)
		out.WriteString(item.line)
		out.WriteByte('\n')
	}
	return out.String()
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func lcs(a, b []string) []op {
	if len(a)*len(b) > 2_000_000 {
		var out []op
		for _, line := range a {
			out = append(out, op{kind: '-', line: line})
		}
		for _, line := range b {
			out = append(out, op{kind: '+', line: line})
		}
		return out
	}
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var out []op
	for i, j := 0, 0; i < len(a) || j < len(b); {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			out = append(out, op{kind: ' ', line: a[i]})
			i++
			j++
		case j < len(b) && (i == len(a) || dp[i][j+1] > dp[i+1][j]):
			out = append(out, op{kind: '+', line: b[j]})
			j++
		default:
			out = append(out, op{kind: '-', line: a[i]})
			i++
		}
	}
	return out
}
