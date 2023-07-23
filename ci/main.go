package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
	"dagger.io/dagger"
)

const GCR_SERVICE_URL = "projects/PROJECTID/locations/us-central1/services/hello-dagger"
const GCR_PUBLISH_ADDRESS = "gcr.io/PROJECTID/cache/hello-dagger"

func main() {
	projectID := os.Getenv("PROJECTID")
	if projectID == "" {
		panic(errors.New("set the PROJECTID env variable"))
	}
	gcrServiceURL := strings.Replace(GCR_SERVICE_URL, "PROJECTID", projectID, 1)
	gcrPublishAddress := strings.Replace(GCR_PUBLISH_ADDRESS, "PROJECTID", projectID, 1)
	// create Dagger client
	ctx := context.Background()
	daggerClient, err := dagger.Connect(ctx, dagger.WithLogOutput(os.Stderr))
	if err != nil {
		panic(err)
	}
	defer daggerClient.Close()

	// get working directory on host
	source := daggerClient.Host().Directory(".", dagger.HostDirectoryOpts{
		Exclude: []string{"ci", "node_modules"},
	})

	// build application
	node := daggerClient.Container(dagger.ContainerOpts{Platform: "linux/amd64"}).
		From("node:16")

	c := node.
		WithDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"cp", "-R", ".", "/home/node"}).
		WithWorkdir("/home/node").
		WithExec([]string{"npm", "install"}).
		WithEntrypoint([]string{"npm", "start"})

	// publish container to Google Container Registry
	addr, err := c.Publish(ctx, gcrPublishAddress)
	if err != nil {
		panic(err)
	}

	// print ref
	fmt.Println("Published at:", addr)

	// create Google Cloud Run client
	gcrClient, err := run.NewServicesClient(ctx)
	if err != nil {
		panic(err)
	}
	defer gcrClient.Close()

	// define service request
	gcrRequest := &runpb.UpdateServiceRequest{
		Service: &runpb.Service{
			Name: gcrServiceURL,
			Template: &runpb.RevisionTemplate{
				Containers: []*runpb.Container{
					{
						Image: addr,
						Ports: []*runpb.ContainerPort{
							{
								Name:          "http1",
								ContainerPort: 8080,
							},
						},
					},
				},
			},
		},
	}

	// update service
	gcrOperation, err := gcrClient.UpdateService(ctx, gcrRequest)
	if err != nil {
		panic(err)
	}

	// wait for service request completion
	gcrResponse, err := gcrOperation.Wait(ctx)
	if err != nil {
		panic(err)
	}

	// print ref
	fmt.Println("Deployment for image", addr, "now available at", gcrResponse.Uri)

}
