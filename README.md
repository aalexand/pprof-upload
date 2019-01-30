# pprof-upload

Binary `pprof-upload` uploads a performance profile in pprof format to
[Stackdriver Profiler UI](https://cloud.google.com/profiler/docs/using-profiler)
for visualization.

## Installation

[Download and install](https://golang.org/doc/install) Go, then run command

```
go get github.com/aalexand/pprof-upload
```

Add your `$GOPATH/bin` to `$PATH` for convenience.

## Usage

You'll need a Google Cloud Platform project to upload and visualize the
profiling data. You can use an existing project you have, or [create a new
one](https://cloud.google.com/resource-manager/docs/creating-managing-projects).
Note the project ID, you'll need it in the upload command.

Make sure the project has the [profiler API
enabled](https://cloud.google.com/profiler/docs/profiling-go#enabling-profiler-api).

Once the prerequisites are completed, run command like:

```
pprof-upload -project_id=your-project-id ~/path/to/profile.pb.gz
```

The command will upload the profile and print out a URL that can be visited to
view the data. You should see something like

![Stackdriver Profiler UI](sample.png?raw=true "Stackdriver Profiler UI")

You can pass more than one profile path to the tool:

```
pprof-upload -project_id=your-project-id ~/path/*.pg.gz
```

In that case the profiles will be merged before being uploaded. The profiles
must be merge-compatible - have the same profile type (e.g. `heap` vs. `cpu`)
and set of metrics. You can also disable the merge like:

```
pprof-upload -merge=false -project_id=your-project-id ~/path/*.pg.gz
```

The profiles must still be merge-compatible but will be uploaded individually
such that the merge will only be done at the viewing time. This is rarely needed
and the main benefit is the ability to use the weight filter in the profiler UI.

Note that Stackdriver Profiler stores data for 30 days, so the profile will be
gone after about a month.

See also [Stackdriver Profiler
quickstart](https://cloud.google.com/profiler/docs/quickstart) on how to enable
continuous production profiling for a service running on Google Cloud Platform.
