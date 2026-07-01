# Pre-baked image for the docker-claude prebuilt definition.
#
# The claude CLI is installed at build time so the sandbox installs nothing at
# run time — the container materializes from this pinned image
# (mework/claude:1.0.0). Build and push with:
#
#   docker build -t mework/claude:1.0.0 definitions/docker-claude
#
FROM node:20-bookworm-slim

# Install the Claude Code CLI globally at build time (no run-time install).
RUN npm install -g @anthropic-ai/claude-code \
    && claude --version

# Non-root run user; the agent runs as an unprivileged user inside the sandbox.
RUN useradd --create-home --shell /bin/bash agent
USER agent
WORKDIR /home/agent

ENTRYPOINT ["claude"]
