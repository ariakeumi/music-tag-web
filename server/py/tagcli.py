#!/usr/bin/env python3
"""Standalone audio-tag CLI used by the Go server.

Read and write audio metadata through the vendored component/music_tag
library. This module intentionally has no Django imports: the Go backend
owns the database, so this CLI only does file I/O and prints JSON on stdout.

Commands:
  read --path <file>              print a single MusicIDS.to_dict() object
  read-batch                      read a JSON list of paths from stdin,
                                  print {"<path>": <dict>, ...}
  write                           read a JSON list of update payloads from
                                  stdin, print {"results": [{"file_full_path",
                                  "ok", "error"}, ...]}
"""
import base64
import json
import os
import re
import sys
import traceback

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
if REPO_ROOT not in sys.path:
    sys.path.insert(0, REPO_ROOT)

from component import music_tag  # noqa: E402
from mutagen.flac import VCFLACDict  # noqa: E402
from mutagen.id3 import ID3, TXXX  # noqa: E402

chinese_pattern = re.compile(r'[\u4e00-\u9fa5]')
english_pattern = re.compile(r'[a-zA-Z]')
japanese_pattern = re.compile(r'[\u0800-\u4e00]')
korean_pattern = re.compile(r'[\uac00-\ud7a3]')
thai_pattern = re.compile(r'[\u0e00-\u0e7f]')


def detect_language(lyrics):
    if not lyrics:
        return "未知"
    counts = {
        "中文": len(re.findall(chinese_pattern, lyrics)),
        "英文": len(re.findall(english_pattern, lyrics)),
        "日文": len(re.findall(japanese_pattern, lyrics)),
        "韩文": len(re.findall(korean_pattern, lyrics)),
        "泰文": len(re.findall(thai_pattern, lyrics)),
    }
    best = max(counts, key=counts.get)
    if counts[best] == 0:
        return "未知"
    return best


# ${xxx} template support: the previous implementation rendered Mako
# expressions; plain identifier substitution covers every template the UI
# generates ("${artist}/${title}/${album}") without a Mako dependency.
TEMPLATE_PATTERN = re.compile(r"\$\{([^${}#]+)\}")


class ConstantTemplate:
    def __init__(self, data):
        self.data = data

    def resolve_data(self, value_maps):
        if not isinstance(self.data, str):
            return self.data
        return TEMPLATE_PATTERN.sub(
            lambda m: str(value_maps.get(m.group(1).strip(), m.group(0))),
            self.data,
        )


class MusicIDS:
    def __init__(self, folder=None, file=None):
        if folder:
            folder = folder.encode("utf-8", "replace").decode()
            self.file = music_tag.load_file(folder)
            self.path = folder
        elif file:
            folder = file.filename.encode("utf-8", "replace").decode()
            self.file = file
            self.path = folder
        self.artwork_w = 0
        self.artwork_h = 0
        self.artwork_size = 0

    @property
    def album_name(self):
        album_name = self.file["album"].value
        album_name = album_name.replace(" ", "")
        if not album_name:
            album_name = "未知专辑"
        return self.file["album"].value

    @property
    def album(self):
        album_name = self.file["album"].value
        album_name = album_name.replace(" ", "")
        if not album_name:
            return ""
        return self.file["album"].value

    @property
    def album_type(self):
        try:
            if isinstance(self.file.mfile.tags, VCFLACDict):
                return self.file.mfile.tags.get("RELEASETYPE")[0]
            elif isinstance(self.file.mfile.tags, ID3):
                return self.file.mfile.tags.get("TXXX:MusicBrainz Album Type").text[0]
            else:
                return ""
        except Exception:
            return ""

    @property
    def album_artist(self):
        return self.file["albumartist"].value

    @property
    def artist_name(self):
        return self.file["artist"].value

    @property
    def artist(self):
        return self.file["artist"].value

    @property
    def year(self):
        try:
            year = self.file["year"].value
        except Exception:
            return 0
        try:
            year = int(year)
        except Exception:
            year_list = str(year).split("-")
            if year_list and year_list[0]:
                try:
                    return int(year_list[0].replace(" ", ""))
                except Exception:
                    return 0
        return year

    @property
    def genre(self):
        genre = self.file["genre"].value
        if genre:
            genre = genre.upper()
        else:
            genre = "未知"
        return genre

    @property
    def comment(self):
        return self.file["comment"].value

    @property
    def lyrics(self):
        return self.file["lyrics"].value

    @property
    def duration(self):
        return round(self.file["#length"].value, 2)

    @property
    def size(self):
        return round(os.path.getsize(self.path) / 1024 / 1024, 2)

    @property
    def suffix(self):
        return self.file["#codec"].value

    @property
    def bit_rate(self):
        return int(self.file["#bitrate"].value / 1000)

    @property
    def track_number(self):
        try:
            return self.file["tracknumber"].value
        except Exception:
            return self.file.mfile.tags["tracknumber"][0]

    @property
    def disc_number(self):
        try:
            return self.file["discnumber"].value
        except Exception:
            return self.file.mfile.tags["discnumber"][0]

    @property
    def title(self):
        return self.file["title"].value

    @property
    def artwork(self):
        try:
            bs64_img = ""
            artwork = self.file["artwork"].values
            if artwork:
                if isinstance(artwork[0], bytes):
                    bs64_img = base64.b64encode(artwork[0]).decode()
                else:
                    zip_img = artwork[0].raw_thumbnail([128, 128])
                    bs64_img = base64.b64encode(zip_img).decode()
                    self.artwork_w = artwork[0].width
                    self.artwork_h = artwork[0].height
                    self.artwork_size = round(len(artwork[0].raw) / 1024 / 1024, 2)
            return "data:image/jpeg;base64," + bs64_img
        except Exception:
            return ""

    @property
    def file_name(self):
        return os.path.basename(self.path)

    @property
    def language(self):
        try:
            if isinstance(self.file.mfile.tags, VCFLACDict):
                language = self.file.mfile.tags.get("LANGUAGE")[0]
            elif isinstance(self.file.mfile.tags, ID3):
                language = self.file.mfile.tags.get("TXXX:LANGUAGE").text[0]
            else:
                language = ""
        except Exception:
            language = ""
        if language:
            return language
        try:
            return detect_language(self.lyrics)
        except Exception:
            return ""

    def var_dict(self):
        return {
            "title": self.title or self.file_name.split(".")[0],
            "artist": self.artist,
            "albumartist": self.album_artist,
            "discnumber": self.disc_number,
            "tracknumber": self.track_number,
            "album": self.album,
            "filename": self.file_name,
        }

    def to_dict(self):
        return {
            "year": self.year,
            "comment": self.comment,
            "lyrics": self.lyrics,
            "duration": self.duration,
            "size": self.size,
            "bit_rate": self.bit_rate,
            "tracknumber": self.track_number,
            "discnumber": self.disc_number,
            "artwork": self.artwork,
            "artwork_w": self.artwork_w,
            "artwork_h": self.artwork_h,
            "artwork_size": self.artwork_size,
            "title": self.title or self.file_name.split(".")[0],
            "artist": self.artist,
            "album": self.album,
            "album_type": self.album_type,
            "genre": self.genre,
            "filename": self.file_name,
            "albumartist": self.album_artist,
            "language": self.language,
        }


def download_image(url):
    import requests

    resp = requests.get(url, headers={"User-Agent": "Mozilla/5.0"}, timeout=15)
    if resp.status_code == 200:
        return resp.content
    return None


def save_music(f, each):
    base_filename = ".".join(os.path.basename(f.filename).split(".")[:-1])
    file_ext = os.path.basename(f.filename).split(".")[-1]

    var_dict = MusicIDS(file=f).var_dict()
    if each.get("title", None):
        if "${" in each["title"]:
            f["title"] = ConstantTemplate(each["title"]).resolve_data(var_dict)
        else:
            f["title"] = each["title"]
    if each.get("artist", None) is not None:
        if "${" in each["artist"]:
            artist = ConstantTemplate(each["artist"]).resolve_data(var_dict)
        else:
            artist = each["artist"]
        artists = artist.split(",")
        f.set("artist", artists)
    if each.get("album", None) is not None:
        if "${" in each["album"]:
            f["album"] = ConstantTemplate(each["album"]).resolve_data(var_dict)
        else:
            f["album"] = each["album"]
    if each.get("albumartist", None):
        if "${" in each["albumartist"]:
            f["albumartist"] = ConstantTemplate(each["albumartist"]).resolve_data(var_dict)
        else:
            f["albumartist"] = each["albumartist"]
    if each.get("discnumber", None):
        if "${" in each["discnumber"]:
            f["discnumber"] = ConstantTemplate(each["discnumber"]).resolve_data(var_dict)
        else:
            try:
                f["discnumber"] = int(each["discnumber"].split("/")[0].strip())
            except Exception:
                f["discnumber"] = 0
    if each.get("tracknumber", None):
        if "${" in each["tracknumber"]:
            f["tracknumber"] = ConstantTemplate(each["tracknumber"]).resolve_data(var_dict)
        else:
            try:
                f["tracknumber"] = int(each["tracknumber"].split("/")[0].strip())
            except Exception:
                f["tracknumber"] = 0
    if each.get("genre", None):
        f["genre"] = each["genre"]
    if each.get("year", None):
        f["year"] = each["year"]
    if each.get("lyrics", None):
        f["lyrics"] = each["lyrics"]
        if each.get("is_save_lyrics_file", False):
            lyrics_file_path = "{}/{}.lrc".format(os.path.dirname(each["file_full_path"]), base_filename)
            with open(lyrics_file_path, "w", encoding="utf-8") as f_lyc:
                f_lyc.write(each["lyrics"])
    else:
        if each.get("lyrics") is not None:
            f.remove_tag("lyrics")
        if each.get("is_save_lyrics_file", False):
            lyrics_file_path = "{}/cover-{}.lrc".format(os.path.dirname(each["file_full_path"]), base_filename)
            if not os.path.exists(lyrics_file_path):
                with open(lyrics_file_path, "w", encoding="utf-8") as f_lyc2:
                    f_lyc2.write(f["lyrics"].value)
    if each.get("comment", None):
        f["comment"] = each["comment"]
    if each.get("album_img", None):
        try:
            img_content = None
            if each["album_img"].startswith("http"):
                img_content = download_image(each["album_img"])
            else:
                img_content = base64.b64decode(each["album_img"])
            if img_content:
                f["artwork"] = img_content
                if each.get("is_save_album_cover", False):
                    format_str = f["artwork"].value.format
                    album_cover_path = "{}/cover-{}.{}".format(
                        os.path.dirname(each["file_full_path"]), f["album"], format_str)
                    if os.path.exists(album_cover_path):
                        os.remove(album_cover_path)
                    if not os.path.exists(album_cover_path):
                        with open(album_cover_path, "wb") as f_img:
                            f_img.write(img_content)
                if len(img_content) / 1024 / 1024 > 5:
                    f["artwork"] = f["artwork"].first.raw_thumbnail([2048, 2048])
        except Exception:
            pass
    else:
        if each.get("is_save_album_cover", False):
            try:
                format_str = f["artwork"].value.format
                album_cover_path = "{}/cover-{}.{}".format(
                    os.path.dirname(each["file_full_path"]), f["album"], format_str)
                if not os.path.exists(album_cover_path):
                    with open(album_cover_path, "wb") as f_img:
                        f_img.write(f["artwork"].value.raw)
            except Exception:
                pass
    if each.get("album_type", None):
        if isinstance(f.mfile.tags, VCFLACDict):
            f.mfile.tags["RELEASETYPE"] = each["album_type"]
        elif isinstance(f.mfile.tags, ID3):
            f.mfile.tags["MusicBrainz Album Type"] = TXXX(encoding=3,
                                                          desc="MusicBrainz Album Type",
                                                          text=each["album_type"])
        else:
            raise Exception("未知的音乐文件类型")
    if each.get("language", None):
        if isinstance(f.mfile.tags, VCFLACDict):
            f.mfile.tags["LANGUAGE"] = each["language"]
        elif isinstance(f.mfile.tags, ID3):
            f.mfile.tags["LANGUAGE"] = TXXX(encoding=3,
                                            desc="LANGUAGE",
                                            text=each["language"])
        else:
            f.mfile.tags["LANGUAGE"] = each["language"]
    f.save()
    # 重命名文件名称
    if each.get("filename", None):
        if "${" in each["filename"]:
            each["filename"] = ConstantTemplate(each["filename"]).resolve_data(var_dict)
        if not each["filename"].endswith(file_ext):
            each["filename"] = "{}.{}".format(each["filename"], file_ext)
        parent_path = os.path.dirname(each["file_full_path"])
        if each["file_full_path"] != "{}/{}".format(parent_path, each["filename"]):
            os.rename(each["file_full_path"], "{}/{}".format(parent_path, each["filename"]))


def cmd_read(path):
    return MusicIDS(path).to_dict()


def cmd_read_batch(paths):
    out = {}
    for path in paths:
        try:
            out[path] = MusicIDS(path).to_dict()
        except Exception as e:
            out[path] = {"error": str(e)}
    return out


def cmd_write(items):
    results = []
    for each in items:
        full_path = each.get("file_full_path", "")
        try:
            f = music_tag.load_file(full_path)
            save_music(f, each)
            results.append({"file_full_path": full_path, "ok": True, "error": ""})
        except Exception as e:
            results.append({
                "file_full_path": full_path,
                "ok": False,
                "error": "{}: {}".format(e.__class__.__name__, e),
            })
    return {"results": results}


def main():
    argv = sys.argv[1:]
    command = argv[0] if argv else ""
    try:
        if command == "read":
            data = cmd_read(argv[argv.index("--path") + 1])
        elif command == "read-batch":
            paths = json.loads(sys.stdin.read())
            data = cmd_read_batch(paths)
        elif command == "write":
            items = json.loads(sys.stdin.read())
            data = cmd_write(items)
        else:
            raise SystemExit("usage: tagcli.py {read --path P | read-batch | write}")
    except SystemExit:
        raise
    except Exception:
        data = {"error": traceback.format_exc()}
    sys.stdout.write(json.dumps(data, ensure_ascii=False))
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
