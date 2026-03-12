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
quiet: true                # Optional: suppress job/step log headers
env:                        # Optional: workflow-level environment variables
  GLOBAL_VAR: value
outputs:                    # Optional: workflow-level outputs (for reusable workflows)
  result: ${{ jobs.build.outputs.status }}
jobs:
  job-name:
    needs: [dependency]    # Optional: jobs that must complete first
    outputs:                # Optional: job-level outputs
      version: ${{ steps.ver.outputs.version }}
    strategy:               # Optional: matrix strategy
      matrix:
        key: ["val1", "val2"]
    env:                    # Optional: job-level environment variables
      JOB_VAR: value
    defaults:               # Optional: default settings for steps
      run:
        shell: bash         # Optional: default shell for steps in this job (sh or bash)
    uses: ./other-workflow  # Optional: reference a reusable workflow (mutually exclusive with steps)
    with:                    # Optional: inputs for uses or matrix expressions
      input_key: value
    steps:
      - id: step-id        # Optional: identifier for referencing outputs
        name: Display Name  # Optional: shown in output
        run: echo "hello"   # Required: shell command to execute
        shell: bash         # Optional: override shell for this step (sh or bash)
        uses: ./action      # Optional: reference an action (mutually exclusive with run)
        with:                # Optional: inputs for action
          input_key: value
        env:                # Optional: step-level environment variables
          STEP_VAR: value
```

### Inputs

Workflows can declare inputs that are passed via the `--input` / `-i` flag at runtime:

```yaml
name: greet
inputs:
  name:
    description: "Who to greet"
    required: true
  greeting:
    description: "Greeting message"
    default: "Hello"
jobs:
  greet:
    steps:
      - run: echo "${{ inputs.greeting }}, ${{ inputs.name }}!"
```

```bash
flow run greet -i name=World                    # uses default greeting "Hello"
flow run greet -i name=Alice -i greeting=Hi     # overrides default
flow run greet                                  # error: required input "name" not provided
```

- `required: true` — the input must be provided; an error occurs if missing and no default is set
- `default: value` — used when the input is not provided
- Access input values with `${{ inputs.key }}` expressions
- Input names must match `^[a-zA-Z0-9_-]+$`
- Use `flow describe <workflow>` to see available inputs for a workflow

### Jobs

Jobs run in parallel whenever possible. Jobs with no dependencies start immediately, while jobs with `needs` wait for their dependencies to complete before starting. This enables patterns like `setup → [lint, test] (parallel) → deploy`.

```yaml
name: build-and-deploy
jobs:
  build:
    steps:
      - run: make build

  lint:
    needs: build
    steps:
      - run: make lint

  test:
    needs: build
    steps:
      - run: make test

  deploy:
    needs: [lint, test]
    steps:
      - run: make deploy
```

In this example, `lint` and `test` run in parallel after `build` completes, and `deploy` waits for both to finish.

The `needs` field accepts a single string or a list:

```yaml
needs: build          # single dependency
needs: [build, lint]  # multiple dependencies
```

If a job fails, all dependent jobs are skipped. Independent jobs continue to run.

Each job's output (stdout/stderr) is buffered and flushed as a unit when the job completes, preventing interleaved output from parallel jobs.

#### Job Outputs

Jobs can declare outputs that are derived from step outputs. These outputs can be referenced by downstream jobs using `${{ needs.<job>.outputs.<key> }}`:

```yaml
name: pipeline
jobs:
  build:
    outputs:
      version: ${{ steps.ver.outputs.version }}
    steps:
      - id: ver
        run: echo "version=1.2.3" >> $FLOW_OUTPUT

  deploy:
    needs: build
    steps:
      - run: echo "Deploying v${{ needs.build.outputs.version }}"
```

#### Reusable Workflows

Jobs can reference other workflows with `uses`, similar to GitHub Actions reusable workflows. Pass inputs with `with`:

```yaml
name: pipeline
jobs:
  build:
    steps:
      - run: make build

  deploy:
    needs: build
    uses: ./deploy
    with:
      version: "${{ needs.build.outputs.version }}"
```

The referenced workflow is loaded from `.flow/workflows/<name>.yaml`. A job cannot have both `uses` and `steps`.

Reusable workflows can declare `outputs` that propagate back to the calling workflow:

```yaml
# .flow/workflows/deploy.yaml
name: deploy
inputs:
  version:
    required: true
outputs:
  result: ${{ jobs.run.outputs.status }}
jobs:
  run:
    outputs:
      status: ${{ steps.do.outputs.status }}
    steps:
      - id: do
        run: echo "status=deployed-${{ inputs.version }}" >> $FLOW_OUTPUT
```

#### Matrix Strategy

Jobs can use `strategy.matrix` to run multiple times with different parameter combinations. Each combination runs in parallel.

**Static values:**

```yaml
name: test
jobs:
  test:
    strategy:
      matrix:
        node: ["16", "18", "20"]
    steps:
      - run: echo "Testing on Node ${{ matrix.node }}"
```

This runs the job 3 times in parallel with `matrix.node` set to `"16"`, `"18"`, and `"20"`.

**Multiple keys (cartesian product):**

```yaml
jobs:
  test:
    strategy:
      matrix:
        os: ["linux", "darwin"]
        arch: ["amd64", "arm64"]
    steps:
      - run: echo "Build for ${{ matrix.os }}/${{ matrix.arch }}"
```

This produces 4 combinations: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`.

**Dynamic values with `fromJson`:**

Use `fromJson()` to expand a JSON array produced by a previous job's output:

```yaml
jobs:
  setup:
    outputs:
      targets: ${{ steps.list.outputs.targets }}
    steps:
      - id: list
        run: echo 'targets=["api","web","worker"]' >> $FLOW_OUTPUT

  deploy:
    needs: setup
    strategy:
      matrix:
        target: ${{ fromJson(needs.setup.outputs.targets) }}
    uses: ./deploy
    with:
      target: ${{ matrix.target }}
```

**Matrix with reusable workflows:**

Matrix works with both `steps` and `uses`:

```yaml
jobs:
  deploy:
    strategy:
      matrix:
        target: ["api", "web"]
    uses: ./deploy
    with:
      target: ${{ matrix.target }}
```

Notes:
- Matrix combinations run in parallel
- Matrix job outputs are **not** propagated to downstream jobs
- Output displays the matrix label: `=== Job: deploy [target=api] ===`
- If any combination fails, the job is marked as failed

### Shell Configuration

By default, steps run via `sh -c`. You can change the shell at the job level (applying to all steps) or at the step level (overriding the job default). Valid values are `sh` and `bash`.

```yaml
name: shell-example
jobs:
  build:
    defaults:
      run:
        shell: bash          # all steps in this job use bash
    steps:
      - run: echo "running in bash"
      - run: echo "running in sh"
        shell: sh            # override for this step only
```

### Steps

Steps within a job execute sequentially and stop on the first failure.

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

For multiline values, use the delimiter syntax (similar to GitHub Actions):

```yaml
steps:
  - id: changelog
    run: |
      echo "body<<EOF" >> $FLOW_OUTPUT
      git log --oneline -5 >> $FLOW_OUTPUT
      echo "EOF" >> $FLOW_OUTPUT

  - run: echo "${{ steps.changelog.outputs.body }}"
```

The format is `KEY<<DELIMITER`, followed by the value lines, followed by `DELIMITER` on its own line. Any string can be used as the delimiter.

- Step `id` must match `^[a-zA-Z0-9-]+$`
- Outputs are scoped to the job; they cannot be referenced across jobs
- Unknown step or key references resolve to an empty string

### Actions

Actions allow you to define reusable step groups in separate files, similar to GitHub Actions composite actions. Action files are placed in `.flow/actions/<action-name>/action.yaml`.

#### Defining an action

```yaml
# .flow/actions/greet/action.yaml
name: greet
description: "Generate a greeting"

inputs:
  name:
    description: "Who to greet"
    required: true
    default: "world"

outputs:
  greeting:
    description: "The generated greeting"

runs:
  steps:
    - id: greet
      name: Generate greeting
      run: echo "greeting=hello ${{ inputs.name }}" >> $FLOW_OUTPUT
```

#### Using an action in a workflow

Reference an action with `uses` and pass inputs with `with`:

```yaml
name: greet-workflow
jobs:
  greet:
    steps:
      - id: my-step
        uses: ./greet
        with:
          name: "Claude"
      - run: echo "${{ steps.my-step.outputs.greeting }}"
```

- `uses: ./<action-name>` — references an action in `.flow/actions/<action-name>/action.yaml`
- `with:` — passes input values to the action (supports `${{ }}` expressions)
- Action outputs are collected from all action steps and exposed as the calling step's outputs
- A step cannot have both `run` and `uses`
- `with` is only valid when `uses` is specified
- Action steps can reference each other's outputs with `${{ steps.<id>.outputs.<key> }}`
- Environment variables are merged: workflow env -> job env -> calling step env -> action step env

### Workflow Outputs

Workflows can declare outputs that map to job outputs. This is primarily useful for reusable workflows, where the calling workflow needs to access results:

```yaml
name: build
outputs:
  version: ${{ jobs.compile.outputs.version }}
jobs:
  compile:
    outputs:
      version: ${{ steps.ver.outputs.version }}
    steps:
      - id: ver
        run: echo "version=1.0.0" >> $FLOW_OUTPUT
```

Workflow output expressions use `${{ jobs.<job>.outputs.<key> }}` syntax.

### Environment Variables

Environment variables can be defined at three levels: workflow, job, and step. Variables are merged in that order, with later levels overriding earlier ones.

```yaml
name: env-example
env:
  APP_NAME: myapp
  LOG_LEVEL: info

jobs:
  build:
    env:
      LOG_LEVEL: debug       # overrides workflow-level LOG_LEVEL
      BUILD_DIR: ./dist
    steps:
      - name: Build
        env:
          BUILD_DIR: ./out    # overrides job-level BUILD_DIR
        run: echo "$APP_NAME $LOG_LEVEL $BUILD_DIR"
        # outputs: myapp debug ./out
```

Merge order: **workflow env** -> **job env** -> **step env** (later levels take precedence).

## Configuration

The `FLOW_ROOT` environment variable sets the root directory (default: `.flow`). Workflows are loaded from `$FLOW_ROOT/workflows/` and actions from `$FLOW_ROOT/actions/`.

## Commands

```
flow list                                List available workflows (alias: ls)
flow run <workflow>                      Run a workflow
flow run <workflow> -i key=value         Pass input values (repeatable)
flow run <workflow> --debug              Run with detailed output (overrides quiet)
flow describe <workflow>                 Show workflow details (inputs, jobs, steps)
flow version                             Show version information
flow version --short                     Show only the version number
```

## Project Layout Example

```
my-project/
  .flow/
    actions/
      greet/
        action.yaml
    workflows/
      build.yaml
      deploy.yaml
      test.yaml
```

## License

See [LICENSE](LICENSE) for details.
