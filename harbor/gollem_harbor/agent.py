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


def _find_binary() -> Path:
    """Locate the pre-built gollem Linux binary.

    Searches in order:
    1. harbor/gollem-linux-amd64 (next to pyproject.toml)
    2. GOLLEM_BINARY env var
    3. Attempts to build it on the fly
    """
    # Check next to pyproject.toml.
    candidate = _PKG_DIR.parent / "gollem-linux-amd64"
    if candidate.exists():
        return candidate

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
        timeout_minutes: int = 30,
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

    async def setup(self, environment: BaseEnvironment) -> None:
        """Upload the pre-built binary instead of compiling from source."""
        binary_path = _find_binary()

        # Ensure CA certificates are available for TLS (some task images lack them).
        # Skip if certs already exist to avoid slow apt-get update.
        await environment.exec(
            command=(
                "if [ ! -f /etc/ssl/certs/ca-certificates.crt ] && [ ! -d /etc/ssl/certs ]; then "
                "("
                "  apt-get update -qq && apt-get install -y -qq ca-certificates"
                "  || apk add --no-cache ca-certificates"
                "  || yum install -y ca-certificates"
                "  || dnf install -y ca-certificates"
                ") > /dev/null 2>&1; "
                "update-ca-certificates > /dev/null 2>&1 || true; "
                "fi"
            )
        )

        # Upload binary to container.
        await environment.exec(command="mkdir -p /usr/local/bin")
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

    def create_run_agent_commands(self, instruction: str) -> list[ExecInput]:
        """Build the shell command to invoke gollem run inside the container."""
        provider, model = self._parse_model_name()

        cmd_parts = [
            "/usr/local/bin/gollem", "run",
            "--provider", provider,
        ]
        if model:
            cmd_parts.extend(["--model", model])
        cmd_parts.extend([
            "--timeout", f"{self._timeout_minutes}m",
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
        }

        for key in [
            "ANTHROPIC_API_KEY",
            "OPENAI_API_KEY",
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
                timeout_sec=self._timeout_minutes * 60 + 60,
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
