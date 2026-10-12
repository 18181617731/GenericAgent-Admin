"""只读验证 GA loop 的固定 API 契约，不安装或修改 GA 核心。"""
from functools import lru_cache
import hashlib
from pathlib import Path
from urllib.request import Request, urlopen

LOOP_REVISION = '6e0c54f6ec72bf18ead57afb50fdd38ce703dba2'
LOOP_SHA256 = 'c4f65ace32d1499e5db10ce69d81545234c29417076d2eab892e2e6879b65bfd'
LOOP_URL = f'https://raw.githubusercontent.com/lsdefine/GenericAgent/{LOOP_REVISION}/agent_loop.py'
LOOP_TIMEOUT_SECONDS = 30


def explicit_loop_path(argv):
    """仅消费真实 Python 文件参数，绝不消费 unittest discover 的参数。"""
    if len(argv) > 1:
        candidate = Path(argv[1])
        if candidate.suffix == '.py' and candidate.is_file():
            return Path(argv.pop(1))
    return None


@lru_cache(maxsize=1)
def loop_source(path=None):
    if path is not None:
        return path.read_text(encoding='utf-8'), str(path)
    request = Request(LOOP_URL, headers={'User-Agent': 'GenericAgent-Admin-Compatibility-Test'})
    with urlopen(request, timeout=LOOP_TIMEOUT_SECONDS) as response:
        source = response.read()
    if hashlib.sha256(source).hexdigest() != LOOP_SHA256:
        raise ValueError('Pinned GA loop compatibility source digest mismatch')
    return source.decode('utf-8'), LOOP_URL
