# flow

A task orchestration CLI tool that runs workflows defined in YAML configuration files, inspired by GitHub Actions.

## Installation

Download a prebuilt binary from the [Releases](https://github.com/longkey1/flow/releases) page.

Available for Linux and macOS (amd64, arm64, arm).

### Build from source

```bash
go install github.com/longkey1/flow@latest
```

## Quick Start

1. Create a workflow file at `.flow/workflows/hello.yaml`:

```yaml
name: hello
jobs:
  greet:
    steps:
      - name: Say hello
        run: echo "Hello, world!"
```

2. Run it:

```bash
flow run hello
```

## Workflow Syntax

Workflow files are written in YAML and placed in the `.flow/workflows/` directory. Both `.yaml` and `.yml` extensions are supported.

### Structure

```yaml
name: workflow-name        # Required
jobs:
  job-name:
    needs: [dependency]    # Optional: jobs that must complete first
    steps:
      - id: step-id        # Optional: identifier for referencing outputs
        name: Display Name  # Optional: shown in output
        run: echo "hello"   # Required: shell command to execute
```

### Jobs

Jobs are executed sequentially in topological order based on their dependencies. When multiple jobs are at the same dependency level, they run in YAML declaration order.

```yaml
name: build-and-deploy
jobs:
  build:
    steps:
      - run: make build

  test:
    needs: build
    steps:
      - run: make test

  deploy:
    needs: test
    steps:
      - run: make deploy
```

The `needs` field accepts a single string or a list:

```yaml
needs: build          # single dependency
needs: [build, lint]  # multiple dependencies
```

If a job fails, all dependent jobs are skipped. Independent jobs continue to run.

### Steps

Each step runs a shell command via `sh -c`. Steps within a job execute sequentially and stop on the first failure.

Steps support interactive input from the terminal (e.g., `read`, `select`):

```yaml
name: confirm
jobs:
  deploy:
    steps:
      - name: Confirm
        run: |
          read -p "Deploy to production? (y/n): " answer
          if [ "$answer" != "y" ]; then
            echo "Aborted."
            exit 1
          fi
      - name: Deploy
        run: ./deploy.sh
```

### Step Outputs

Steps can produce outputs that are consumed by subsequent steps within the same job.

Write `KEY=VALUE` lines to the file at `$FLOW_OUTPUT`:

```yaml
name: outputs-example
jobs:
  build:
    steps:
      - id: version
        name: Determine version
        run: echo "tag=v1.2.3" >> $FLOW_OUTPUT

      - name: Use version
        run: echo "Building ${{ steps.version.outputs.tag }}"
```

- Step `id` must match `^[a-zA-Z0-9-]+$`
- Outputs are scoped to the job; they cannot be referenced across jobs
- Unknown step or key references resolve to an empty string

## Configuration

An optional `.flow.yaml` file in the project root configures the workflows directory:

```yaml
dir: .flow  # default
```

Workflows are loaded from `<dir>/workflows/`. If `.flow.yaml` is not present, the default `.flow/workflows/` is used.

## Commands

```
flow run <workflow>     Run a workflow
flow version            Show version information
flow version --short    Show only the version number
```

## Project Layout Example

```
my-project/
  .flow.yaml              # Optional configuration
  .flow/
    workflows/
      build.yaml
      deploy.yaml
      test.yaml
```

## License

See [LICENSE](LICENSE) for details.
