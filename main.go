// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//            http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"gopkg.in/urfave/cli.v1"
	"log"
	"os"
	"time"
)

var logerr *log.Logger

func main() {
	defer timeTrack(time.Now(), "Total time")
	var filename string
	var credentialsPath string
	var orgId string
	var projectId string
	app := cli.NewApp()
	app.Name = "policygopher"
	app.UsageText = "policygopher [options]"
	app.Usage = "Dumps all members roles and permissions for a GCP organization"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "file",
			Value:       "member_role_permissions.csv",
			Usage:       "csv file output",
			Destination: &filename,
		},
		cli.StringFlag{
			Name:        "org, o",
			Usage:       "Organization ID",
			Destination: &orgId,
		},
		cli.StringFlag{
			Name:        "project, p",
			Usage:       "Project ID, used to find Org ID if unspecified",
			Destination: &projectId,
		},
		cli.StringFlag{
			Name:        "credentials, c",
			Usage:       "credentials.json, used to find Org ID if Org ID or ProjectID are unspecified",
			EnvVar:      "GOOGLE_APPLICATION_DEFAULT",
			Destination: &credentialsPath,
		},
	}

	app.Action = func(c *cli.Context) error {
		return printToCsv(filename, credentialsPath, orgId, projectId)
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func printToCsv(filename string, credentialsPath string, orgId string, projectId string) error {
	ctx := context.Background()
	if _, err := os.Stat(filename); err == nil {
		log.Printf("Fils %s found, skipping export roles", filename)
		return nil
	}
	f, err := os.Create(fmt.Sprintf("tmp.%s", filename))
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(f)
	_, err = fmt.Fprintf(writer, "%s,%s,%s,%s,%s\n", "Resource", "Type", "Member", "Role", "Permission")
	if err != nil {
		return err
	}
	logerr = log.New(os.Stderr, "Error: ", 0)

	resman, err := NewResourceManager(ctx, credentialsPath, orgId, projectId)
	if err != nil {
		return err
	}

	allRows, err := resman.GetAllPolicyRows()
	if err != nil {
		return err
	}
	defer timeTrack(time.Now(), "Printing CSV")
	fmt.Println("Printing CSV")
	for _, row := range *allRows {
		if err := row.Print(writer, resman); err != nil {
			logerr.Printf("%v\n", err)
		}
	}
	if err := os.Rename(fmt.Sprintf("tmp.%s", filename), filename); err != nil {
		return errors.New(fmt.Sprintf("Unable to move tmp.%s to %s: %v", filename, filename, err))
	}
	if err := writer.Flush(); err != nil {
		return errors.New(fmt.Sprintf("Error flushing writer: %v", err))
	}
	if err := f.Close(); err != nil {
		return errors.New(fmt.Sprintf("Error closing file: %v", err))
	}
	return nil
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
}
