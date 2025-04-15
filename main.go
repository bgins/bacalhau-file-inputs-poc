package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/bacalhau-project/bacalhau/pkg/models"
	"github.com/bacalhau-project/bacalhau/pkg/publicapi/apimodels"
	client "github.com/bacalhau-project/bacalhau/pkg/publicapi/client/v2"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start Bacalhau client
	httpClient := client.NewHTTPClient("http://localhost:1234")
	api := client.NewAPI(httpClient)

	// Prepare job
	job := getJob()

	// Submit job
	resp, err := api.Jobs().Put(ctx, &apimodels.PutJobRequest{
		Job: &job,
	})
	if err != nil {
		log.Fatalf("Failed to submit job: %v", err)
	}
	fmt.Printf("Job submitted successfully! ID: %s\n", resp.JobID)

	// Poll job
	for {
		fmt.Println("Checking job status...")

		jobInfo, err := api.Jobs().Get(ctx, &apimodels.GetJobRequest{
			JobID:   resp.JobID,
			Include: "executions",
		})
		if err != nil {
			log.Fatalf("Failed to get job status: %v", err)
		}

		stateType := jobInfo.Job.State.StateType
		if stateType == models.JobStateTypeRunning {
			fmt.Println("Job is running")
		} else if stateType == models.JobStateTypeCompleted {
			fmt.Println("Job completed successfully!")

			outputPath, err := retrieveOutputs(ctx, api, resp.JobID)
			if err != nil {
				fmt.Printf("unable to retrieve results: %s", err)
			}
			fmt.Printf("Results available in: %s\n", outputPath)

			break
		} else if stateType == models.JobStateTypeFailed {
			fmt.Printf("Job failed: %s\n", jobInfo.Job.State.Message)
			break
		} else if stateType == models.JobStateTypeStopped {
			fmt.Println("Job was stopped")
			break
		}

		jsonData, _ := json.MarshalIndent(jobInfo.Job, "", "  ")
		fmt.Println(string(jsonData))

		time.Sleep(1 * time.Second)
	}
}

func getJob() models.Job {
	return models.Job{
		Name:      "copy-file-contents",
		Namespace: "default",
		Type:      "batch",
		Count:     1,
		Priority:  50,
		Meta:      make(map[string]string),
		Labels:    make(map[string]string),
		Tasks: []*models.Task{
			{
				Name: "copy-file-contents",
				Engine: &models.SpecConfig{
					Type: "docker",
					Params: map[string]any{
						"Image": "ubuntu:latest",
						"Entrypoint": []string{
							"/bin/sh",
							"-c",
							"cat /tmp/input.txt > /outputs/output.txt",
						},
					},
				},
				InputSources: []*models.InputSource{&models.InputSource{
					Source: &models.SpecConfig{
						Type: "localDirectory",
						Params: map[string]any{
							"SourcePath": getInputsPath(),
							"ReadWrite":  true,
						},
					},
					Target: "/tmp",
				}},
				Publisher: &models.SpecConfig{
					Type: "local",
				},
				ResultPaths: []*models.ResultPath{
					{
						Name: "outputs",
						Path: "/outputs",
					},
				},
				ResourcesConfig: &models.ResourcesConfig{
					CPU:    "0.5",
					Memory: "100m",
					GPU:    "0",
				},
			},
		},
	}
}

// Get absolute path for inputs
func getInputsPath() string {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get current working directory: %v", err)
	}

	return filepath.Join(cwd, "inputs")
}

func retrieveOutputs(ctx context.Context, api client.API, jobID string) (string, error) {
	results, err := api.Jobs().Results(ctx, &apimodels.ListJobResultsRequest{
		JobID: jobID,
	})
	if err != nil {
		fmt.Printf("error retrieving results: %s", err)
	}
	resultsURL := results.Items[0].Params["URL"].(string)

	// Prepare target file
	resultsDir := "./outputs"
	tarballPath := filepath.Join(resultsDir, fmt.Sprintf("%s.tar.gz", jobID))
	out, err := os.Create(tarballPath)
	if err != nil {
		return "", fmt.Errorf("error creating file: %s", err.Error())
	}
	defer out.Close()

	// Get data from Bacalhau
	resp, err := http.Get(resultsURL)
	if err != nil {
		return "", fmt.Errorf("error making GET request: %s", err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	// Write the body to the target
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("error writing to file: %s", err.Error())
	}

	// Extract the tar.gz file
	outputPath := filepath.Join(resultsDir, jobID)
	err = extractTarGz(tarballPath, outputPath)
	if err != nil {
		return "", fmt.Errorf("error extracting tar.gz file: %s", err.Error())
	}

	return outputPath, nil
}

func extractTarGz(src, dst string) error {
	file, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dst, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}
