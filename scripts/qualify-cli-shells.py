#!/usr/bin/env python3
"""Opt-in native CLI completion in private Bash/Zsh PTYs; stdlib only."""
import fcntl
import hashlib
import json
import platform
import sys
import os
from pathlib import Path
import pty
import re
import select
import shlex
import shutil
import signal
import struct
import subprocess
import termios
import time
import uuid


def main():
    assert os.environ.get('PDS_QUALIFY_CLI_SHELLS') == 'true', 'Explicit isolated shell qualification required'
    assert sys.platform == 'linux' and platform.machine() in {'x86_64', 'amd64'}, 'Native Linux AMD64 required'
    assert shutil.which('bash') and shutil.which('zsh'), 'Bash and Zsh are required; no shell is installed automatically'
    candidate = Path('.local/cli-build/pds-ass-linux-amd64')
    assert candidate.is_file() and not candidate.is_symlink()
    binary = candidate.resolve()
    checksum = hashlib.sha256(binary.read_bytes()).hexdigest()
    checksum_text = Path('.local/cli-build/SHA256SUMS').read_text(encoding='utf-8')
    assert checksum + '  pds-ass-linux-amd64' in checksum_text.splitlines()
    root = Path('.local/cli-shells', str(uuid.uuid4())).resolve()
    root.mkdir(parents=True, mode=0o700)
    (root / 'bin').mkdir(mode=0o700)
    (root / 'bin' / 'pds-ass').symlink_to(binary)
    completions = root / 'completions'
    completions.mkdir(mode=0o700)
    def personal_metadata():
        home = Path.home()
        paths = {home / '.bash_history', home / '.zsh_history', *home.glob('.zcompdump*')}
        return {str(path): (path.lstat().st_ino, path.lstat().st_size, path.lstat().st_mtime_ns) for path in paths if path.exists()}
    before_personal = personal_metadata()
    results = []
    report = {'result': 'failed', 'stage': 'setup', 'id': root.name, 'binarySha256': checksum, 'shells': results, 'realAccountAuthorizationQualified': False, 'persistentUserInstallationQualified': False, 'fishQualified': False, 'releasePublished': False}
    def receipt():
        path = root / 'report.json'
        path.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
        path.chmod(0o600)

    class Shell:
        def __init__(self, name):
            self.name = name
            self.buffer = b''
            self.transcript = b''
            self.reaped = False
            self.pid, self.fd = pty.fork()
            if self.pid == 0:
                os.chdir(root)
                env = {'PATH': str(root / 'bin') + ':/usr/bin:/bin', 'TERM': 'xterm-256color', 'LC_ALL': 'C', 'INPUTRC': '/dev/null', 'HISTFILE': '/dev/null', 'XDG_CONFIG_HOME': str(root / 'config')}
                args = ['--noprofile', '--norc', '-i'] if name == 'bash' else ['-d', '-f', '-i']
                os.execve(shutil.which(name), [name, *args], env)
            fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack('HHHH', 24, 160, 0, 0))

        def send(self, value):
            os.write(self.fd, value.encode())

        def until(self, marker):
            deadline = time.monotonic() + 10
            while marker not in self.buffer:
                assert time.monotonic() < deadline, 'Shell marker timed out'
                if not select.select([self.fd], [], [], 0.1)[0]:
                    continue
                data = os.read(self.fd, 65536)
                assert data, 'Shell ended before marker'
                self.buffer += data
                self.transcript += data
                assert len(self.transcript) < 1 << 20, 'Shell output exceeded qualification budget'
            end = self.buffer.index(marker) + len(marker)
            result, self.buffer = self.buffer[:end], self.buffer[end:]
            return result

        def close(self):
            status = None
            try:
                try:
                    self.send('\x15exit\n')
                except OSError:
                    pass
                deadline = time.monotonic() + 3
                while time.monotonic() < deadline:
                    pid, status = os.waitpid(self.pid, os.WNOHANG)
                    if pid:
                        self.reaped = True
                        break
                    status = None
                    time.sleep(0.02)
                if not self.reaped:
                    try:
                        os.killpg(self.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    _, status = os.waitpid(self.pid, 0)
                    self.reaped = True
                return os.waitstatus_to_exitcode(status)
            finally:
                if not self.reaped:
                    try:
                        os.killpg(self.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    os.waitpid(self.pid, 0)
                    self.reaped = True
                os.close(self.fd)
                path = root / (self.name + '-transcript.txt')
                path.write_bytes(self.transcript)
                path.chmod(0o600)

    for name in ['bash', 'zsh']:
        report['stage'] = name + '_completion'
        receipt()
        generated = subprocess.run([str(binary), 'completion', name], capture_output=True, timeout=10)
        assert generated.returncode == 0 and not generated.stderr
        path = completions / ('pds-ass.bash' if name == 'bash' else '_pds-ass')
        path.write_bytes(generated.stdout)
        path.chmod(0o600)
        shell = Shell(name)
        try:
            if name == 'bash':
                setup = r'''set +o history; set -o emacs; PS1='cli> '; source FILE; _capture_cli_buffer() { printf '\n__CLI_CAPTURE_BEGIN__%s__CLI_CAPTURE_END__\n' "$READLINE_LINE"; READLINE_LINE=; READLINE_POINT=0; }; bind -x '"\C-x\C-g":_capture_cli_buffer';'''
                setup = setup.replace('FILE', shlex.quote(str(path)))
            else:
                setup = r'''SAVEHIST=0; HISTSIZE=0; PROMPT='cli> '; fpath=(DIR $fpath); autoload -Uz compinit; compinit -d CACHE -D -i; bindkey -e; _capture_cli_buffer() { printf '\n__CLI_CAPTURE_BEGIN__%s__CLI_CAPTURE_END__\n' "$BUFFER"; BUFFER=; CURSOR=0; zle redisplay; }; zle -N _capture_cli_buffer; bindkey '^X^G' _capture_cli_buffer;'''
                setup = setup.replace('DIR', shlex.quote(str(completions))).replace('CACHE', shlex.quote(str(root / 'unused-compdump')))
            setup += r''' printf '\n%s%s\n' '__CLI_' 'READY__' '''
            shell.send(setup + '\n')
            shell.until(b'__CLI_READY__')
            cases = []
            for typed, expected in [('pds-ass sta', 'pds-ass status'), ('pds-ass status --dep', 'pds-ass status --deployment'), ('pds-ass deploy --idem', 'pds-ass deploy --idempotency-key'), ('pds-ass definitely-unmatched', 'pds-ass definitely-unmatched')]:
                shell.send(typed + '\t\x18\x07')
                data = shell.until(b'__CLI_CAPTURE_END__')
                found = re.search(rb'__CLI_CAPTURE_BEGIN__(.*?)__CLI_CAPTURE_END__', data, re.S)
                assert found, 'No captured editor buffer'
                actual = found[1].decode().strip()
                cases.append({'typed': typed, 'actual': actual, 'expected': expected, 'passed': actual == expected})
            version = subprocess.run([shutil.which(name), '--version'], capture_output=True, text=True, encoding='utf-8', timeout=10)
            assert version.returncode == 0
            results.append({'shell': name, 'version': version.stdout.splitlines()[0], 'cases': cases})
            assert all(case['passed'] for case in cases), 'Shell completion buffer did not match its contract'
        finally:
            exit_code = shell.close()
            report.setdefault('shellExits', []).append({'shell': name, 'exit': exit_code, 'reaped': shell.reaped})
            receipt()
            assert exit_code == 0 and shell.reaped, 'Owned shell did not exit cleanly'
    assert personal_metadata() == before_personal, 'Personal shell history/cache metadata changed'
    assert hashlib.sha256(binary.read_bytes()).hexdigest() == checksum
    assert not (root / 'unused-compdump').exists()
    assert Path('.local/cli-build/SHA256SUMS').read_text(encoding='utf-8') == checksum_text
    report.update(result='passed', stage='complete', completedAt=time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()), binariesUnchanged=True, privateCompletionCacheAbsent=True, personalShellMetadataUnchanged=True, allOwnedShellsReaped=True, qualifierSha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), fishAvailable=shutil.which('fish') is not None)
    receipt()
    print(json.dumps(report))

if __name__ == '__main__':
    try:
        main()
    except (AssertionError, OSError, subprocess.SubprocessError) as error:
        print('CLI shell qualification failed: ' + str(error), file=sys.stderr)
        raise SystemExit(1)
