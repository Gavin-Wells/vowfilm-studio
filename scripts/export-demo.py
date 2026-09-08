#!/usr/bin/env python3
"""Export a completed local film as a portable, credential-free demo."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("project")
parser.add_argument("--name", default="joy-together")
args = parser.parse_args()
if not re.fullmatch(r"[A-Za-z0-9_-]+", args.name):
    parser.error("name must be a plain filename stem")
root = Path(__file__).resolve().parent.parent
p = json.loads((root / "data/projects.json").read_text())[args.project]
if p["status"] != "completed" or not p.get("filmUrl"):
    parser.error("project has no completed film")
source = root / "data" / p["id"]
dest = root / "public/demo"
dest.mkdir(parents=True, exist_ok=True)
movie = dest / f"{args.name}.mp4"
subprocess.run([
    "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
    "-i", str(source / Path(p["filmUrl"]).name), "-map", "0:v:0", "-map", "0:a:0",
    "-c:v", "libx264", "-preset", "slow", "-crf", "24", "-pix_fmt", "yuv420p",
    "-threads", "2", "-c:a", "aac", "-ar", "48000", "-ac", "2", "-b:a", "160k",
    "-movflags", "+faststart", str(movie),
], check=True)
subprocess.run(["ffmpeg", "-v", "error", "-i", str(movie), "-f", "null", "-"], check=True)
info = json.loads(subprocess.check_output([
    "ffprobe", "-v", "error", "-show_streams", "-show_format", "-of", "json", str(movie)
]))
video = next(s for s in info["streams"] if s["codec_type"] == "video")
audio = next(s for s in info["streams"] if s["codec_type"] == "audio")
if abs(float(info["format"]["duration"]) - p["duration"]) > .05:
    raise RuntimeError("export duration changed")
if movie.stat().st_size >= 25 * 1024 * 1024:
    raise RuntimeError("demo exceeds Sites single-asset size limit")

def save_json(name, value):
    (dest / name).write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")

manifest = {
    "file": movie.name, "sha256": hashlib.sha256(movie.read_bytes()).hexdigest(),
    "bytes": movie.stat().st_size, "duration_seconds": float(info["format"]["duration"]),
    "width": video["width"], "height": video["height"], "fps": video["avg_frame_rate"],
    "video_codec": video["codec_name"], "audio_codec": audio["codec_name"],
    "audio_sample_rate": int(audio["sample_rate"]), "audio_channels": audio["channels"],
    "generated_artifact": True, "fictional_demo": p["demo"], "source_project": p["id"],
    "llm_model": p["llmModel"], "video_model": p["videoModel"],
    "music_source": p.get("musicSource"), "full_decode": "passed",
}
if p.get("musicFile"):
    music = dest / f"{args.name}-score.mp3"
    shutil.copyfile(source / p["musicFile"], music)
    p["musicUrl"] = f"/demo/{music.name}"
    manifest["music_file"] = music.name
    manifest["music_sha256"] = hashlib.sha256(music.read_bytes()).hexdigest()
for shot in p["shots"]:
    if shot.get("thumbnailUrl"):
        thumb = Path(shot["thumbnailUrl"]).name
        archive_thumb = f"{args.name}-{thumb}"
        shutil.copyfile(source / thumb, dest / archive_thumb)
        shot["thumbnailUrl"] = f"/demo/{archive_thumb}"
    for key in ["taskId", "videoFile", "videoUrl", "error", "reserved"]:
        shot.pop(key, None)
p["posterUrl"] = p["shots"][min(7, len(p["shots"])-1)].get("thumbnailUrl", "")
p["filmUrl"] = f"/demo/{movie.name}"
p["assets"] = []
p["events"] = []
p["message"] = f'{p["duration"]} 秒{p["title"]}成片已归档'
p.pop("musicTaskId", None)
p.pop("musicFile", None)
save_json("project.json", p)
save_json("film-manifest.json", manifest)
quality = json.loads((source / "quality-report.json").read_text())
quality["full_decode"] = "passed"
save_json("quality-report.json", quality)
print(json.dumps(manifest, ensure_ascii=False, indent=2))
