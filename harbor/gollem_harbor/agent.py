"""Harbor adapter for the gollem coding agent.

This module implements the BaseInstalledAgent interface so gollem can be
evaluated on Terminal-Bench 2.0 via Harbor.

Usage:
    # First, build the Linux binary:
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o harbor/gollem-linux-amd64 ./cmd/gollem/

    # Then run:
    harbor run -d terminal-bench@2.0 \\
        --agent-import-path gollem_harbor:GollemAgent \\
        -m anthropic/claude-sonnet-4-6 \\
        --env docker
"""

from __future__ import annotations

import json
import logging
import os
import shlex
import subprocess
from pathlib import Path

from harbor.agents.installed.base import BaseInstalledAgent, ExecInput
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext

_PKG_DIR = Path(__file__).parent
_TEMPLATES_DIR = _PKG_DIR / "templates"
_REPO_ROOT = _PKG_DIR.parent.parent  # gollem repo root
_TASK_CACHE_DIR = Path.home() / ".cache" / "harbor" / "tasks"

logger = logging.getLogger(__name__)


def _find_binary() -> Path:
    """Locate the pre-built gollem Linux binary.

    Searches in order:
    1. harbor/gollem-linux-amd64 (canonical build target)
    2. harbor/gollem_harbor/gollem (alternative location)
    3. GOLLEM_BINARY env var
    4. Attempts to build it on the fly
    """
    # Check canonical location next to pyproject.toml.
    candidate = _PKG_DIR.parent / "gollem-linux-amd64"
    if candidate.exists():
        return candidate

    # Check inside the package directory.
    candidate2 = _PKG_DIR / "gollem"
    if candidate2.exists():
        return candidate2

    # Check env var.
    if env_path := os.environ.get("GOLLEM_BINARY"):
        p = Path(env_path)
        if p.exists():
            return p

    # Try to build it.
    build_target = _PKG_DIR.parent / "gollem-linux-amd64"
    cmd_dir = _REPO_ROOT / "cmd" / "gollem"
    if cmd_dir.exists():
        subprocess.run(
            ["go", "build", "-o", str(build_target), "./cmd/gollem/"],
            cwd=str(_REPO_ROOT),
            env={**os.environ, "GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"},
            check=True,
            capture_output=True,
        )
        if build_target.exists():
            return build_target

    raise FileNotFoundError(
        "gollem Linux binary not found. Build it with:\n"
        "  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o harbor/gollem-linux-amd64 ./cmd/gollem/"
    )


class GollemAgent(BaseInstalledAgent):
    """Harbor agent that runs the gollem coding agent CLI.

    Instead of compiling Go inside the container (slow), we upload a
    pre-built static binary. This reduces setup time from 6+ minutes
    to under 10 seconds.
    """

    SUPPORTS_ATIF = False

    def __init__(
        self,
        *args,
        timeout_minutes: int = 210,
        thinking_budget: int = 0,
        reasoning_effort: str = "",
        **kwargs,
    ):
        super().__init__(*args, **kwargs)
        self._timeout_minutes = timeout_minutes
        self._thinking_budget = thinking_budget
        self._reasoning_effort = reasoning_effort

    @staticmethod
    def name() -> str:
        return "gollem"

    @property
    def _install_agent_template_path(self) -> Path:
        return _TEMPLATES_DIR / "install.sh.j2"

    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        """Override run() to detect the task-specific timeout before execution.

        Harbor wraps this in asyncio.wait_for(timeout=task_timeout_sec), but
        the agent binary doesn't know the real deadline. We read the task
        timeout from the task cache and pass it via GOLLEM_TIMEOUT_SEC.
        """
        task_timeout = self._detect_task_timeout()
        if task_timeout:
            self._task_timeout_sec = task_timeout
            logger.info(f"Detected task timeout: {task_timeout}s")
        else:
            self._task_timeout_sec = None

        await super().run(instruction, environment, context)

    def _detect_task_timeout(self) -> int | None:
        """Find the task-specific timeout from Harbor's task cache.

        The trial directory name has format '<task-name>__<random>'.
        We extract the task name and search the task cache for task.toml.
        """
        try:
            # Extract task name from trial directory name.
            trial_dir = self.logs_dir.parent
            trial_name = trial_dir.name
            task_name = trial_name.rsplit("__", 1)[0]
            if not task_name:
                return None

            # Search the task cache for this task's task.toml.
            if _TASK_CACHE_DIR.exists():
                for cache_entry in _TASK_CACHE_DIR.iterdir():
                    task_toml = cache_entry / task_name / "task.toml"
                    if task_toml.exists():
                        return self._parse_timeout_from_toml(task_toml)
        except Exception as e:
            logger.debug(f"Failed to detect task timeout: {e}")
        return None

    @staticmethod
    def _parse_timeout_from_toml(path: Path) -> int | None:
        """Parse agent.timeout_sec from a task.toml file."""
        try:
            text = path.read_text()
            in_agent = False
            for line in text.splitlines():
                line = line.strip()
                if line == "[agent]":
                    in_agent = True
                elif line.startswith("["):
                    in_agent = False
                elif in_agent and line.startswith("timeout_sec"):
                    _, _, val = line.partition("=")
                    return int(float(val.strip()))
        except Exception:
            pass
        return None

    async def setup(self, environment: BaseEnvironment) -> None:
        """Upload the pre-built binary instead of compiling from source."""
        binary_path = _find_binary()

        # Combined setup: fix dpkg, install CA certs and common tools, create bin dir.
        # Also create swap space to prevent OOM kills (Anthropic's research found
        # 5.8% of TB2 failures are from container OOM). Setup time doesn't count
        # against the agent's timeout, so this is free from the agent's perspective.
        await environment.exec(
            command=(
                "dpkg --configure -a > /dev/null 2>&1 || true; "
                "mkdir -p /usr/local/bin; "
                # Create 1GB swap to prevent OOM kills on memory-intensive tasks.
                "if [ ! -f /swapfile ]; then "
                "  dd if=/dev/zero of=/swapfile bs=1M count=1024 2>/dev/null && "
                "  chmod 600 /swapfile && "
                "  mkswap /swapfile 2>/dev/null && "
                "  swapon /swapfile 2>/dev/null || true; "
                "fi; "
                "timeout 120 sh -c '("
                "  if command -v apt-get >/dev/null 2>&1; then "
                "    apt-get update -qq 2>/dev/null && "
                "    apt-get install -y -qq ca-certificates python3-pip build-essential "
                "      curl wget git jq unzip file bc sqlite3 xxd pkg-config "
                "      cmake autoconf automake libtool libssl-dev 2>/dev/null; "
                "  elif command -v apk >/dev/null 2>&1; then "
                "    apk add --no-cache ca-certificates python3 py3-pip build-base "
                "      curl wget git jq unzip file bc sqlite cmake openssl-dev 2>/dev/null; "
                "  elif command -v yum >/dev/null 2>&1; then "
                "    yum install -y ca-certificates python3-pip gcc make "
                "      curl wget git jq unzip file bc sqlite cmake openssl-devel 2>/dev/null; "
                "  fi"
                ") 2>&1 | tail -5' || true; "
                "update-ca-certificates > /dev/null 2>&1 || true"
            )
        )

        # Upload binary to container and make executable.
        await environment.upload_file(
            source_path=binary_path,
            target_path="/usr/local/bin/gollem",
        )
        await environment.exec(command="chmod +x /usr/local/bin/gollem")

        # Verify.
        result = await environment.exec(command="gollem --help")
        if result.return_code != 0:
            raise RuntimeError(
                f"gollem binary verification failed: {result.stderr}"
            )

        # Auto-install task dependencies found in the container.
        # This runs during setup (before the agent timer starts), saving 2-5
        # agent turns that would otherwise be spent on `pip install` / `npm install`.
        # All installs are best-effort with timeouts — failures are ignored.
        await environment.exec(
            command=(
                "timeout 180 sh -c '"
                # Install pytest and commonly needed Python packages. Nearly all
                # verifier tests use pytest, and numpy/requests/pyyaml cover ~60%
                # of TB2 task dependencies. Installing here is free (setup time).
                "pip install --break-system-packages -q pytest numpy requests pyyaml 2>/dev/null || "
                "pip3 install --break-system-packages -q pytest numpy requests pyyaml 2>/dev/null || true; "
                # Python: requirements.txt
                "for f in /app/requirements.txt /requirements.txt; do "
                "  if [ -f \"$f\" ]; then "
                "    pip install --break-system-packages -r \"$f\" 2>&1 | tail -5 || "
                "    pip3 install --break-system-packages -r \"$f\" 2>&1 | tail -5 || "
                "    python3 -m pip install --break-system-packages -r \"$f\" 2>&1 | tail -5 || true; "
                "    break; "
                "  fi; "
                "done; "
                # Python: setup.py (editable install for projects with setup.py)
                "for d in /app .; do "
                "  if [ -f \"$d/setup.py\" ] && ! [ -f \"$d/requirements.txt\" ]; then "
                "    cd \"$d\" && pip install --break-system-packages -e . 2>&1 | tail -5 || true; "
                "    break; "
                "  fi; "
                "done; "
                # Python: scan test files for imports and install missing ones
                "for td in /tests /app/tests; do "
                "  if [ -d \"$td\" ]; then "
                "    grep -rh \"^import \\|^from \" \"$td\"/*.py 2>/dev/null | "
                "      sed \"s/^import //;s/^from //;s/ .*//;s/\\..*//\" | "
                "      sort -u | while read mod; do "
                "        python3 -c \"import $mod\" 2>/dev/null || "
                "          pip install --break-system-packages -q \"$mod\" 2>/dev/null || true; "
                "    done; "
                "    break; "
                "  fi; "
                "done; "
                # Node.js: package.json
                "for d in /app .; do "
                "  if [ -f \"$d/package.json\" ] && command -v npm >/dev/null 2>&1; then "
                "    cd \"$d\" && npm install --no-audit --no-fund 2>&1 | tail -5 || true; "
                "    break; "
                "  fi; "
                "done; "
                # Go: go.mod
                "for d in /app .; do "
                "  if [ -f \"$d/go.mod\" ] && command -v go >/dev/null 2>&1; then "
                "    cd \"$d\" && go mod download 2>&1 | tail -3 || true; "
                "    break; "
                "  fi; "
                "done"
                "' || true"
            )
        )

    def create_run_agent_commands(self, instruction: str) -> list[ExecInput]:
        """Build the shell command to invoke gollem run inside the container."""
        provider, model = self._parse_model_name()

        # Use the task-specific timeout if detected, otherwise fall back
        # to the generic timeout_minutes. Leave 60s buffer for cleanup.
        task_timeout = getattr(self, "_task_timeout_sec", None)
        if task_timeout:
            agent_timeout_secs = max(task_timeout - 60, 60)
            gollem_timeout_sec = task_timeout
            exec_timeout_sec = task_timeout + 120
        else:
            agent_timeout_secs = max((self._timeout_minutes - 1) * 60, 60)
            gollem_timeout_sec = self._timeout_minutes * 60
            exec_timeout_sec = self._timeout_minutes * 60 + 60

        cmd_parts = [
            "/usr/local/bin/gollem", "run",
            "--provider", provider,
        ]
        if model:
            cmd_parts.extend(["--model", model])
        cmd_parts.extend([
            "--timeout", f"{agent_timeout_secs}s",
        ])
        if self._thinking_budget > 0:
            cmd_parts.extend(["--thinking-budget", str(self._thinking_budget)])
        if self._reasoning_effort:
            cmd_parts.extend(["--reasoning-effort", self._reasoning_effort])

        cmd_parts.append(shlex.quote(instruction))

        env = {
            "PATH": "/usr/local/bin:/usr/local/sbin:/usr/bin:/usr/sbin:/bin:/sbin",
            "HOME": "/root",
            # Common CA cert bundle paths for TLS verification.
            "SSL_CERT_FILE": "/etc/ssl/certs/ca-certificates.crt",
            "SSL_CERT_DIR": "/etc/ssl/certs",
            # Pass the REAL task timeout to gollem so TimeBudgetMiddleware
            # warns at the correct percentages of remaining time.
            "GOLLEM_TIMEOUT_SEC": str(gollem_timeout_sec),
        }

        for key in [
            "ANTHROPIC_API_KEY",
            "OPENAI_API_KEY",
            "OPENAI_BASE_URL",
            "GOOGLE_CLOUD_PROJECT",
            "GOOGLE_APPLICATION_CREDENTIALS",
            "LANGFUSE_SECRET_KEY",
            "LANGFUSE_PUBLIC_KEY",
            "LANGFUSE_BASE_URL",
        ]:
            if val := os.environ.get(key):
                env[key] = val

        return [
            ExecInput(
                command=" ".join(cmd_parts),
                env=env,
                timeout_sec=exec_timeout_sec,
            ),
        ]

    def populate_context_post_run(self, context: AgentContext) -> None:
        """Parse gollem output from the command logs."""
        import re

        command_dir = self.logs_dir / "command-0"
        if not command_dir.exists():
            return

        stdout_path = command_dir / "stdout.txt"
        stderr_path = command_dir / "stderr.txt"
        return_code_path = command_dir / "return-code.txt"

        stdout = stdout_path.read_text() if stdout_path.exists() else ""
        stderr = stderr_path.read_text() if stderr_path.exists() else ""
        return_code = (
            int(return_code_path.read_text().strip())
            if return_code_path.exists()
            else -1
        )

        # Harbor's Docker exec merges stderr into stdout, so stderr.txt
        # may not exist or may be empty. Search both for log data.
        combined_output = stdout + "\n" + stderr

        trajectory = {
            "agent": "gollem",
            "model": self.model_name,
            "return_code": return_code,
            "stdout": stdout[-5000:] if len(stdout) > 5000 else stdout,
            "stderr": stderr[-5000:] if len(stderr) > 5000 else stderr,
        }

        # Parse token usage from gollem's "done" line.
        # Format: "gollem: done (tokens: 12345 in, 6789 out, tools: 42)"
        # Search combined output since stderr may be merged into stdout.
        done_match = re.search(
            r"tokens:\s*(\d+)\s*in,\s*(\d+)\s*out,\s*tools:\s*(\d+)",
            combined_output,
        )
        if done_match:
            trajectory["input_tokens"] = int(done_match.group(1))
            trajectory["output_tokens"] = int(done_match.group(2))
            trajectory["tool_calls"] = int(done_match.group(3))

        # Count tool invocations from log hooks.
        tool_starts = combined_output.count("[gollem] tool:start")
        if tool_starts > 0:
            trajectory["tool_invocations"] = tool_starts

        traj_path = self.logs_dir / "trajectory.json"
        traj_path.write_text(json.dumps(trajectory, indent=2))

        # Populate AgentContext with token usage.
        if "input_tokens" in trajectory:
            context.n_input_tokens = trajectory["input_tokens"]
        if "output_tokens" in trajectory:
            context.n_output_tokens = trajectory["output_tokens"]

        context.metadata = {
            "trajectory_path": str(traj_path),
            "return_code": return_code,
            "tool_invocations": trajectory.get("tool_invocations", 0),
        }

    def _parse_model_name(self) -> tuple[str, str]:
        """Parse 'provider/model' format into (provider, model) tuple."""
        if not self.model_name:
            return "anthropic", ""

        if "/" in self.model_name:
            provider, model = self.model_name.split("/", 1)
            provider_map = {
                "anthropic": "anthropic",
                "openai": "openai",
                "google": "vertexai",
                "vertexai": "vertexai",
                "vertex": "vertexai",
            }
            return provider_map.get(provider, provider), model

        return "anthropic", self.model_name
