package nomination

import (
	"testing"
)

func TestGeneratedMergePatch(t *testing.T) {
	workloadCountMap := map[string]int{
		"workload1": 2,
		"workload2": 1,
	}
	b, _ := generatedMergePatch(workloadCountMap)
	t.Logf(b)
}
