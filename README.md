# bacalhau-file-inputs-poc

A short example showing how to use local filesystem inputs to a Bacalhau job.

### Run

Install:

- Docker Engine (https://docs.docker.com/engine/install/) or Docker Desktop (https://docs.docker.com/desktop/)
- Bacalhau: https://github.com/bacalhau-project/bacalhau?tab=readme-ov-file#getting-started

Start Docker.

Start Bacalhau and allow the `inputs` directory in this repo (fill in the absolute path from your system).

```sh
bacalhau serve --orchestrator --compute --config Compute.AllowListedLocalPaths="/path/to/inputs:rw"
```

Run a job from the root of the repo.

```
go run .
```

The contents of `inputs/input.txt` should be copied into an `output.txt` file in the outputs directory.
