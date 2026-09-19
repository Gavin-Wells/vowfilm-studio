"""Align the original script to measured ASR word times, never character-count timing."""
import difflib
import json
import re
import sys
from pathlib import Path
from faster_whisper import WhisperModel


def align(audio, script_file, output):
    script = Path(script_file).read_text(encoding="utf-8").strip()
    normalize = lambda text: "".join(c for c in text if c.isalnum())
    model = WhisperModel("small", device="cpu", compute_type="int8", cpu_threads=4)
    segments, info = model.transcribe(audio, language="zh", word_timestamps=True,
        vad_filter=True, initial_prompt="简体中文。婚礼故事旁白。" + script[:100])
    measured = []
    for segment in segments:
        for word in segment.words:
            measured.extend((c, word.start, word.end) for c in normalize(word.word))
    target = normalize(script)
    spoken = "".join(c[0] for c in measured)
    matcher = difflib.SequenceMatcher(None, target, spoken, autojunk=False)
    if not measured or matcher.ratio() < 0.75:
        raise ValueError("旁白与原稿匹配不足，请检查漏句或合成结果")
    mapping = {}
    for tag, a, b, c, d in matcher.get_opcodes():
        if tag in ("equal", "replace"):
            for index in range(a, b):
                mapping[index] = min(d-1, c+index-a) if d>c else max(0,c-1)
    for index in range(len(target)):
        mapping.setdefault(index, mapping.get(index-1, 0))
    cues, position = [], 0
    for clause in re.split(r"[，。！？；、\n]+", script):
        text = clause.strip()
        count = len(normalize(text))
        if not count:
            continue
        start, end = measured[mapping[position]][1], measured[mapping[position+count-1]][2]
        position += count
        if end <= start:
            raise ValueError("无法可靠对齐旁白，请检查原始语音")
        cues.append(dict(id=f"C{len(cues)+1:02}", start=start, end=end, text=text))
    for i in range(len(cues)-1):
        cues[i]["end"] = min(cues[i]["end"], cues[i+1]["start"])
    Path(output).write_text(json.dumps(dict(cues=cues, method="actual-audio ASR word alignment",
        duration=info.duration, script_similarity=matcher.ratio()), ensure_ascii=False, indent=2), encoding="utf-8")


if __name__ == "__main__":
    align(*sys.argv[1:])
