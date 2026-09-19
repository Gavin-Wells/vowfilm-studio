package studio

import "testing"

func TestPackCommerceGenerateGroups(t *testing.T) {
	shots := []Shot{
		{ID: "S01", Duration: 4, Prompt: "镜1"},
		{ID: "S02", Duration: 5, Prompt: "镜2"},
		{ID: "S03", Duration: 6, Prompt: "镜3"},
		{ID: "S04", Duration: 5, Prompt: "镜4"},
	}
	packed, err := packCommerceGenerateGroups(shots, 15)
	if err != nil {
		t.Fatal(err)
	}
	if commerceGenerateUnitCount(packed) != 2 {
		t.Fatal("want 2 generate units", packed)
	}
	if !packed[0].GenerateUnit || packed[0].GenerateSeconds != 15 || packed[1].GenerateUnit {
		t.Fatal("first group leader", packed[0])
	}
	if !packed[3].GenerateUnit || packed[3].GenerateSeconds != 5 {
		t.Fatal("second group leader", packed[3])
	}
}

func TestVideoModelMaxSegment(t *testing.T) {
	if videoModelMaxSegmentSeconds("starnet/minimax-h3") != 15 {
		t.Fatal("h3")
	}
	if videoModelMaxSegmentSeconds("volcengine/doubao-seedance-2-0") != 30 {
		t.Fatal("seedance")
	}
}
