"""Impure helpers: tool presence and subprocess execution."""
import shutil, subprocess


def check_tools(names):
    """Return the subset of names not found on PATH."""
    return [n for n in names if shutil.which(n) is None]


def run(cmd):
    """Run a command, raising with captured stderr on failure."""
    p = subprocess.run(cmd, capture_output=True, text=True)
    if p.returncode != 0:
        raise RuntimeError(f"{cmd[0]} failed ({p.returncode}):\n{p.stderr}")
    return p
