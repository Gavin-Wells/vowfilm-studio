package wedding

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"vowfilm/server/internal/domain"
)

func TestSourceSnapshotAndDownloadAreVerbatim(t *testing.T) {
	if err := VerifySource(); err != nil {
		t.Fatal(err)
	}
	data, err := SourcesZIP()
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, f := range z.File {
		if !strings.HasPrefix(f.Name, "upstream/") {
			continue
		}
		r, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(b, Read(strings.TrimPrefix(f.Name, "upstream/"))) {
			t.Fatal("changed download", f.Name)
		}
		count++
	}
	if count != 26 {
		t.Fatalf("missing upstream files: %d", count)
	}
	for step, files := range map[int][]string{2: {"SKILL.md", "references/workflow.md", "references/writing.md", "assets/writing-pack-guide.md", "assets/story-intake.md"}, 6: {"SKILL.md", "references/workflow.md", "references/visuals.md"}} {
		for _, name := range files {
			if !strings.Contains(SystemPrompt(step), string(Read(name))) {
				t.Fatalf("step %d abridged %s", step, name)
			}
		}
	}
}

func TestWorkflowRequiresCurrentVersionAndExplicitAcceptance(t *testing.T) {
	w := New()
	art := []domain.WeddingArtifact{{ID: "a", SHA256: "abc", File: "facts.txt"}}
	if err := Prepare(w, 2, art); err == nil {
		t.Fatal("skipped collection")
	}
	for n := 1; n <= 14; n++ {
		if err := Prepare(w, n, art); err != nil {
			t.Fatal(err)
		}
		v := w.Steps[n-1].Version
		if err := Approve(w, n, "stale", "producer", "user", "reviewed"); err == nil {
			t.Fatal("stale approved")
		}
		if err := Approve(w, n, v, Required(n)[0], "user", ""); err == nil {
			t.Fatal("no evidence approved")
		}
		if n == 3 {
			if err := Approve(w, n, v, "producer", "user", "reviewed"); err == nil {
				t.Fatal("producer substituted for couple")
			}
		}
		if n == 14 {
			if err := Approve(w, n, v, "couple", "user", "couple feedback"); err == nil {
				t.Fatal("couple preceded producer")
			}
		}
		for _, role := range Required(n) {
			if err := Approve(w, n, v, role, "user", "explicit feedback for this version"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !w.Completed {
		t.Fatal("complete acceptance not reflected")
	}
	if err := Reopen(w, 5, nil, "旁白音色改变"); err != nil {
		t.Fatal(err)
	}
	if w.Completed || w.CurrentStep != 5 || w.Steps[3].Status != "confirmed" {
		t.Fatal("wrong invalidation boundary")
	}
	for _, s := range w.Steps[4:] {
		if len(s.Approvals) != 0 || len(s.Artifacts) == 0 {
			t.Fatal("rework must retain files and invalidate approvals")
		}
	}
	if len(w.History) != 10 {
		t.Fatal("lost audit history")
	}
}
