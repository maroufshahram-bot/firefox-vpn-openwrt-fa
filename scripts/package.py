#!/usr/bin/env python3
"""Build an ordered archive with normalized timestamps and ownership."""
import gzip
import hashlib
import pathlib
import sys
import tarfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
TARGETS = ("armv7", "arm64", "amd64", "mips", "mipsle")

def package(target):
    if target not in TARGETS:
        raise SystemExit("Unknown target")
    source = ROOT / "build" / target
    if (source / "TARGET").read_text().strip() != target:
        raise SystemExit("Target metadata mismatch")
    binary = source / "bin/firefox-vpn-core"
    header = binary.read_bytes()[:20]
    if len(header) < 20 or header[:4] != b"\x7fELF" or header[5] not in (1, 2):
        raise SystemExit("Not a valid ELF executable")
    machine = int.from_bytes(header[18:20], "little" if header[5] == 1 else "big")
    wanted = {"armv7": (1, 1, 40), "arm64": (2, 1, 183), "amd64": (2, 1, 62),
              "mips": (1, 2, 8), "mipsle": (1, 1, 8)}[target]
    if (header[4], header[5], machine) != wanted:
        raise SystemExit("ELF CPU target or endianness mismatch")
    expected = hashlib.sha256(binary.read_bytes()).hexdigest()
    if (source / "SHA256SUMS").read_text().split()[0] != expected:
        raise SystemExit("Binary checksum mismatch")
    version = (ROOT / "VERSION").read_text().strip()
    output = ROOT / "dist"
    output.mkdir(exist_ok=True)
    filename = output / f"firefox-vpn-openwrt-v{version}-linux-{target}.tar.gz"
    with filename.open("wb") as raw, gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=0) as gz:
        with tarfile.open(fileobj=gz, mode="w") as archive:
            for path in [source] + sorted(source.rglob("*")):
                if path.is_symlink():
                    raise SystemExit(f"Unexpected symlink: {path}")
                name = pathlib.Path("firefox-vpn-openwrt") / path.relative_to(source)
                info = archive.gettarinfo(str(path), arcname=str(name))
                info.uid = info.gid = info.mtime = 0
                info.uname = info.gname = ""
                info.mode = 0o755 if path.is_dir() or path.suffix == ".sh" or path == binary else 0o644
                if path.is_file():
                    with path.open("rb") as stream:
                        archive.addfile(info, stream)
                else:
                    archive.addfile(info)
    digest = hashlib.sha256(filename.read_bytes()).hexdigest()
    filename.with_name(filename.name + ".sha256").write_text(f"{digest}  {filename.name}\n")
    print(filename.name)

if __name__ == "__main__":
    package(sys.argv[1])
