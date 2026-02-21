"""Harbor adapter for the gollem coding agent.

This module implements the BaseInstalledAgent interface so gollem can be
evaluated on Terminal-Bench 2.0 via Harbor.

Usage:
    harbor run -d terminal-bench@2.0 \\
        --agent-import-path gollem_harbor:GollemAgent \\
        -m anthropic/claude-sonnet-4-5 \\
        --env docker
"""

from __future__ import annotations

import json
import os
import shlex
from pathlib import Path

from harbor.agents.installed.base import BaseInstalledAgent, ExecInput
from harbor.models.agent.context import AgentContext

_TEMPLATES_DIR = Path(__file__).parent / "templates"


class GollemAgent(BaseInstalledAgent):
    """Harbor agent that runs the gollem coding agent CLI."""

    SUPPORTS_ATIF = False  # We emit a simpler trajectory format.

    def __init__(
        self,
        *args,
        timeout_minutes: int = 30,
        thinking_budget: int = 0,
        **kwargs,
    ):
        super().__init__(*args, **kwargs)
        self._timeout_minutes = timeout_minutes
        self._thinking_budget = thinking_budget

    @staticmethod
    def name() -> str:
        return "gollem"

    @property
    def _install_agent_template_path(self) -> Path:
        return _TEMPLATES_DIR / "install.sh.j2"

    def create_run_agent_commands(self, instruction: str) -> list[ExecInput]:
        """Build the shell command to invoke gollem run inside the container."""
        # Determine provider from model name (e.g. "anthropic/claude-sonnet-4-5").
        provider, model = self._parse_model_name()

        # Build the gollem run command.
        cmd_parts = [
            "gollem", "run",
            "--provider", provider,
        ]
        if model:
            cmd_parts.extend(["--model", model])
        cmd_parts.extend([
            "--timeout", f"{self._timeout_minutes}m",
        ])
        if self._thinking_budget > 0:
            cmd_parts.extend(["--thinking-budget", str(self._thinking_budget)])

        # The instruction is the prompt.
        cmd_parts.append(shlex.quote(instruction))

        # Environment variables for API keys and Go path.
        env = {
            "PATH": "/usr/local/go/bin:/root/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
            "HOME": "/root",
        }

        # Pass through API keys from the host environment.
        for key in [
            "ANTHROPIC_API_KEY",
            "OPENAI_API_KEY",
            "GOOGLE_CLOUD_PROJECT",
            "GOOGLE_APPLICATION_CREDENTIALS",
        ]:
            if val := os.environ.get(key):
                env[key] = val

        return [
            ExecInput(
                command=" ".join(cmd_parts),
                env=env,
                timeout_sec=self._timeout_minutes * 60 + 60,  # +1min buffer
            ),
        ]

    def populate_context_post_run(self, context: AgentContext) -> None:
        """Parse gollem output from the command logs."""
        # Read stdout from the last command execution.
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

        # Build a simple trajectory.
        trajectory = {
            "agent": "gollem",
            "model": self.model_name,
            "return_code": return_code,
            "stdout": stdout,
            "stderr": stderr,
        }

        # Parse token usage from stderr (gollem prints it there).
        # Format: "gollem: done (tokens: N in, M out, tools: K)"
        for line in stderr.splitlines():
            if "tokens:" in line and "done" in line:
                try:
                    parts = line.split("tokens:")[1].strip().rstrip(")")
                    tokens = parts.split(",")
                    for t in tokens:
                        t = t.strip()
                        if t.endswith("in"):
                            trajectory["input_tokens"] = int(t.split()[0])
                        elif t.endswith("out"):
                            trajectory["output_tokens"] = int(t.split()[0])
                except (IndexError, ValueError):
                    pass

        # Write trajectory.
        traj_path = self.logs_dir / "trajectory.json"
        traj_path.write_text(json.dumps(trajectory, indent=2))

        # Set the agent output as the final answer.
        context.output = stdout.strip() if stdout.strip() else None
        context.trajectory_path = traj_path

    def _parse_model_name(self) -> tuple[str, str]:
        """Parse 'provider/model' format into (provider, model) tuple.

        Harbor passes model names as 'provider/model' (e.g.
        'anthropic/claude-sonnet-4-5'). We split this into the provider
        and model parts that gollem expects.
        """
        if not self.model_name:
            return "anthropic", ""

        if "/" in self.model_name:
            provider, model = self.model_name.split("/", 1)
            # Map common Harbor provider names to gollem provider names.
            provider_map = {
                "anthropic": "anthropic",
                "openai": "openai",
                "google": "vertexai",
                "vertexai": "vertexai",
                "vertex": "vertexai",
            }
            return provider_map.get(provider, provider), model

        return "anthropic", self.model_name
