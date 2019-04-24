# policygopher
Dump org-wide Google Cloud IAM policies, members, roles, and permissions, to csv

## Why?
Troubleshooting who has what permission, and what role on what resource gave them that permission, can be time consuming. The idea here is to dump everything in a CSV so you can grep, use Sheets, or load it into a DB to query.

## Usage:

    NAME:
       policygopher - Dumps all members roles and permissions for a GCP organization
    
    USAGE:
       policygopher [options]
    
    VERSION:
       0.0.0
    
    COMMANDS:
         help, h  Shows a list of commands or help for one command
    
    GLOBAL OPTIONS:
       --file value                   csv file output (default: "member_role_permissions.csv")
       --org value, -o value          Organization ID
       --project value, -p value      Project ID, used to find Org ID if unspecified
       --credentials value, -c value  credentials.json, used to find Org ID if Org ID or ProjectID are unspecified [$GOOGLE_APPLICATION_DEFAULT]
       --help, -h                     show help
       --version, -v                  print the version

## State:
* Usable, WIP
* This will list direct members of an IAM policy, groups and users
* This will not traverse the groups members
    * I.E. If policy 'foo' has the members user:Jane, group:Dev, and Sally is in group:Dev, Jane and Dev will be listed in the CSV, not Sally

## TODO:
* list specific minimum necessary permissions to run this (resourcemanager view + IAM view, etc)
* add tests
* traverse group memberships