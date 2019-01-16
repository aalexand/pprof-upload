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
	projectID = flag.String("project_id", "", "cloud project ID where the profile will be uploaded")
	apiAddr   = flag.String("api_addr", "cloudprofiler.googleapis.com:443", "profiler API address")
)

// readProfiles reads profile files in pprof format at specified paths and
// merges them. Input profiles must have the same type (e.g. heap vs. CPU).
func readProfiles(fnames []string) (*profile.Profile, error) {
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
	p, err := profile.Merge(ps)
	if err != nil {
		return nil, fmt.Errorf("failed to merge profiles: %v", err)
	}
	return p, nil
}

const scope = "https://www.googleapis.com/auth/monitoring.write"

// uploadProfile uploads the specified profile to Stackdriver Profiler. It
// returns the name of the profile type detected from the profile content.
func uploadProfile(ctx context.Context, p *profile.Profile, service, version string) (string, error) {
	pt, err := guessType(p)
	if err != nil {
		return "", err
	}
	var bb bytes.Buffer
	if err := p.Write(&bb); err != nil {
		return "", err
	}
	opts := []option.ClientOption{
		option.WithEndpoint(*apiAddr),
		option.WithScopes(scope),
	}
	conn, err := gtransport.Dial(ctx, opts...)
	if err != nil {
		return "", err
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
		return "", err
	}
	return pt.String(), nil
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
		os.Exit(2)
	}

	p, err := readProfiles(flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()

	// Reset the profile timestamp to current time so that it's recent in the UI.
	// Otherwise the uploaded profile might have its timestamp even older than 30
	// days which is the maximum query window in the UI.
	p.TimeNanos = now.UnixNano()

	// Assign a unique version based on the current timestamp so that the
	// uploaded profile can be filtered in the UI using the version filter.
	const service = "uploaded-profiles"
	version := now.Format(time.RFC3339)

	ctx := context.Background()
	ptype, err := uploadProfile(ctx, p, service, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Profile uploaded, click the link below to view it.")
	// Escape the ":" character in the profiler URL as it has special meaning.
	version = strings.Replace(version, ":", "~3a", -1)
	fmt.Printf("https://console.cloud.google.com/profiler/%s;type=%s;version=%s?project=%s\n", service, ptype, version, *projectID)
}
