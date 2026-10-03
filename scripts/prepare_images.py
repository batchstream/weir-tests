#!/usr/bin/env python3
"""Explicitly download the two immutable local test images before a tagged run."""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


pins = json.loads((Path(__file__).resolve().parent.parent / 'versions.json').read_text())
endpoint = os.environ.get('DOCKER_HOST') or subprocess.check_output(
    ['docker', 'context', 'inspect', '--format', '{{.Endpoints.docker.Host}}'], text=True, timeout=10).strip()
if not endpoint.startswith('unix://'):
    raise ValueError('image preparation requires a local Unix Docker socket')
env = dict(os.environ)
for name in ('DOCKER_CONTEXT', 'DOCKER_HOST', 'DOCKER_CONFIG'):
    env.pop(name, None)
# These are public images. An isolated empty auth config avoids accessing user
# credentials or waiting for a desktop credential helper.
with tempfile.TemporaryDirectory(prefix='weir-public-images-') as temporary:
    (Path(temporary) / 'config.json').write_text('{"auths":{}}\n')
    for key in ('mongodb_image', 'elasticsearch_image'):
        image = pins[key]
        if re.fullmatch(r'[a-zA-Z0-9./_-]+@sha256:[0-9a-f]{64}', image) is None:
            raise ValueError('requires immutable image digest: ' + key)
        subprocess.run(['docker', '--config', temporary, '--host', endpoint, 'pull', image], env=env, check=True, timeout=600)
