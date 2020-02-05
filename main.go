// Binary pprof-upload uploads a performance profile in pprof format to
// Stackdriver Profiler UI for visualization.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/pprof/profile"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	pb "google.golang.org/genproto/googleapis/devtools/cloudprofiler/v2"
)

var (
	projectID = flag.String("project_id", "", "cloud project ID where the profile will be uploaded (Required)")
	service   = flag.String("service_name", "uploaded-profiles", "name of service for uploaded profiles")
	version   = flag.String("service_version", "", "version of service for uploaded profiles")
	apiAddr   = flag.String("api_addr", "cloudprofiler.googleapis.com:443", "profiler API address")
	merge     = flag.Bool("merge", true, "when false, upload individual profiles")
)

// readProfiles reads profile files in pprof format at specified paths.
// Input profiles must have the same type (e.g. heap vs. CPU).
func readProfiles(fnames []string) ([]*profile.Profile, error) {
	var ps []*profile.Profile
	for _, fname := range fnames {
		file, err := os.Open(fname)
		if err != nil {
			return nil, fmt.Errorf("failed to read profile: %v", err)
		}
		defer file.Close()
		p, err := profile.Parse(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read profile %s: %v", fname, err)
		}
		ps = append(ps, p)
	}
	return ps, nil
}

const scope = "https://www.googleapis.com/auth/monitoring.write"

// uploadProfile uploads the specified profile to Stackdriver Profiler.
func uploadProfile(ctx context.Context, p *profile.Profile, service, version string) error {
	pt, err := guessType(p)
	if err != nil {
		return err
	}
	var bb bytes.Buffer
	if err := p.Write(&bb); err != nil {
		return err
	}
	opts := []option.ClientOption{
		option.WithEndpoint(*apiAddr),
		option.WithScopes(scope),
	}
	conn, err := gtransport.Dial(ctx, opts...)
	if err != nil {
		return err
	}
	client := pb.NewProfilerServiceClient(conn)
	req := pb.CreateOfflineProfileRequest{
		Parent: "projects/" + *projectID,
		Profile: &pb.Profile{
			ProfileType: pt,
			Deployment: &pb.Deployment{
				ProjectId: *projectID,
				Target:    service,
				Labels: map[string]string{
					"version": version,
				},
			},
			ProfileBytes: bb.Bytes(),
		},
	}
	_, err = client.CreateOfflineProfile(ctx, &req)
	if err != nil {
		return err
	}
	return nil
}

func guessType(p *profile.Profile) (pb.ProfileType, error) {
	var types []string
	for _, st := range p.SampleType {
		switch st.Type {
		case "cpu":
			return pb.ProfileType_CPU, nil
		case "wall":
			return pb.ProfileType_WALL, nil
		case "space", "inuse_space":
			return pb.ProfileType_HEAP, nil
		}
		types = append(types, st.Type)
	}
	return pb.ProfileType_PROFILE_TYPE_UNSPECIFIED, fmt.Errorf("failed to guess profile type from sample types %v", types)
}

func main() {
	flag.Parse()

	if *projectID == "" || len(flag.Args()) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: pprof-upload -project_id=PROJECT_ID FILE...")
		flag.PrintDefaults()
		os.Exit(2)
	}

	ps, err := readProfiles(flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if !*merge {
		fmt.Fprintf(os.Stderr, "Will upload %d profile(s)\n", len(ps))
	}

	// Merge the profiles even if we plan to upload them individually, that is to
	// make sure that they can be merged.
	p, err := profile.Merge(ps)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: profiles cannot be merged (different profile types?): %v\n", err)
		os.Exit(1)
	}

	ptype, err := guessType(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *merge {
		ps = []*profile.Profile{p}
	}
	now := time.Now()

	// Assign a unique version based on the current timestamp so that the
	// uploaded profile can be filtered in the UI using the version filter.
	serviceVersion := *version
	if len(serviceVersion) == 0 {
		serviceVersion = now.Format(time.RFC3339)
	}

	ctx := context.Background()
	for i, p := range ps {
		// Reset the profile timestamp to ~current time so that it's recent in the
		// UI. Otherwise the uploaded profile might have its timestamp even older
		// than 30 days which is the maximum query window in the UI. Make the profile
		// timestamp unique in microseconds since it's used as a key in the profiler.
		p.TimeNanos = now.UnixNano() + int64(i)*1000
		if err := uploadProfile(ctx, p, *service, *version); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Uploaded %d profile(s)\n", i+1)
	}

	// Escape the ":" character in the profiler URL as it has special meaning.
	serviceVersion = strings.Replace(serviceVersion, ":", "~3a", -1)
	fmt.Printf("https://console.cloud.google.com/profiler/%s;type=%s;version=%s?project=%s\n", *service, ptype, serviceVersion, *projectID)
}
