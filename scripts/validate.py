#!/usr/bin/env python3
"""Local checks for maintained integration files; not an upstream source audit."""
import json
import pathlib
import re
import subprocess
import sys
import yaml

ROOT = pathlib.Path(__file__).resolve().parents[1]
errors = []
for path in (ROOT / 'packaging').glob('*.sh'):
    result = subprocess.run(['sh', '-n', str(path)], capture_output=True, text=True)
    if result.returncode:
        errors.append(result.stderr)
for path in (ROOT / '.github').rglob('*.yml'):
    try:
        value = yaml.safe_load(path.read_text())
        if not isinstance(value, dict):
            errors.append(f'{path}: not a mapping')
        if 'ISSUE_TEMPLATE' in str(path) and path.name != 'config.yml':
            ids = [item['id'] for item in value['body'] if 'id' in item]
            if len(ids) != len(set(ids)):
                errors.append(f'{path}: duplicate form IDs')
    except Exception as exc:
        errors.append(f'{path}: {exc}')
paths = list(ROOT.glob('*.md')) + list((ROOT / 'docs').glob('*.md'))
for path in paths:
    for link in re.findall(r'\]\(([^)]+)\)', path.read_text()):
        if re.match(r'^[a-z]+://|^#', link):
            continue
        target = link.split('#')[0]
        if not (path.parent / target).exists():
            errors.append(f'{path.relative_to(ROOT)}: missing link {target}')
# Explicitly check that the extracted source really matches the existing distribution.
import hashlib
import tarfile
with tarfile.open(ROOT / 'downloads/firefox-vpn-openwrt-fa-v0.1.0.tar.gz') as archive:
    prefix = 'firefox-vpn-openwrt-fa/upstream/core/'
    for member in archive.getmembers():
        if member.isfile() and member.name.startswith(prefix):
            relative = member.name[len(prefix):]
            local = ROOT / 'src/core' / relative
            if not local.exists() or local.read_bytes() != archive.extractfile(member).read():
                errors.append(f'Upstream source differs: {relative}')
if errors:
    print('\n'.join(errors), file=sys.stderr)
    raise SystemExit(1)
print('Shell syntax, YAML, local documentation links and upstream provenance: passed.')
