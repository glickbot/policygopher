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
	"golang.org/x/oauth2/google"
	v1beta1 "google.golang.org/api/cloudresourcemanager/v1beta1"
	v2beta1 "google.golang.org/api/cloudresourcemanager/v2beta1"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/iam/v1"
	"io/ioutil"
	"os"
)

type Row struct {
	Resource string
	Type     string
	Role     string
	Member   string
}

func (r *Row) Print(writer *bufio.Writer, rm *resourceManager) error {
	var permissions []string
	var err error
	permissions, err = rm.GetRolePermissions(r)
	if err != nil {
		logerr.Printf("Error getting permissions for %s\n", r.Role)
		permissions = []string{"UNKNOWN"}
	}
	for _, p := range permissions {
		_, err := fmt.Fprintf(writer, "%s,%s,%s,%s,%s\n", r.Resource, r.Type, r.Member, r.Role, p)
		if err != nil {
			break
		}
	}
	return err
}

type resourceManager struct {
	ctx     context.Context
	v1      *v1beta1.Service
	v2      *v2beta1.Service
	orgId   string
	service *iam.Service
	roleMap map[string]*iam.Role
}

func NewResourceManager(ctx context.Context, credentialsPath string, orgId string, projectId string) (*resourceManager, error) {
	v1, err := v1beta1.NewService(ctx)
	if err != nil {
		return &resourceManager{}, err
	}
	v2, err := v2beta1.NewService(ctx)
	if err != nil {
		return &resourceManager{}, err
	}
	service, err := iam.NewService(ctx)
	if err != nil {
		return &resourceManager{}, err
	}
	r := &resourceManager{
		ctx:     ctx,
		v1:      v1,
		v2:      v2,
		orgId:   orgId,
		service: service,
		roleMap: make(map[string]*iam.Role, 0),
	}
	if r.orgId == "" {
		fmt.Println("OrgId not specified, checking by ProjectId")
		var p string
		if projectId == "" {
			fmt.Println("ProjectId not specified, getting ProjectId from credentials")
			p, err = r.getProjectIdFromCredentials(credentialsPath)
			if err != nil {
				return &resourceManager{}, errors.New(
					fmt.Sprintf("Unable to identify OrgId, please specify on CLI or gcloud credentials: %v",
						err))
			}
			projectId = p
		}
		err := r.GetOrgIdFromProjectId(projectId)
		if err != nil {
			return &resourceManager{}, errors.New(fmt.Sprintf("Error getting OrgId from ProjectId %s: %v", p, err))
		}
	}
	return r, nil
}

func (r *resourceManager) GetRolePermissions(row *Row) ([]string, error) {
	role, err := r.GetRole(row)
	if err != nil {
		return []string{}, errors.New(fmt.Sprintf("row: %#v, %v", row, err))
	}
	return role.IncludedPermissions, nil
}

func (r *resourceManager) GetRole(row *Row) (*iam.Role, error) {
	var try_uri string
	var role *iam.Role
	var err error
	if row.Type == "project" || row.Type == "organization" {
		try_uri = fmt.Sprintf("%ss/%s/%s", row.Type, row.Resource, row.Role)
		role, err = r._getRoleByUri(try_uri)
		if err == nil {
			//fmt.Printf("[%s] <== [%s]\n", try_uri, row.Role)
			return role, nil
		}
	}
	role, err = r._getRoleByUri(row.Role)
	if err != nil {
		return &iam.Role{}, err
	}
	if try_uri != "" {
		//fmt.Printf("[%s] ==> [%s]\n", try_uri, row.Role)
	}
	return role, nil
}

func (r *resourceManager) _getRoleByUri(uri string) (*iam.Role, error) {
	var role *iam.Role
	var err error
	if role, ok := r.roleMap[uri]; ok {
		return role, nil
	}
	role, err = r.service.Roles.Get(uri).Do()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("uri[%s]: %v", uri, err))
	}
	r.roleMap[uri] = role
	return role, err
}

func (r *resourceManager) getProjectIdFromCredentials(credentialsPath string) (string, error) {
	credentials, err := google.FindDefaultCredentials(r.ctx, compute.ComputeScope)
	if err == nil && credentials.ProjectID != "" {
		fmt.Printf("Project ID found from default credentials: %s\n", credentials.ProjectID)
		return credentials.ProjectID, nil
	}
	if credentialsPath == "" {
		return "", errors.New("unable to get application default credentials, please specify credentials json")
	}
	_, err = os.Stat(credentialsPath)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Unable to stat credential file %s: %v", credentialsPath, err))
	}
	data, err := ioutil.ReadFile(credentialsPath)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Error opening %s: %v", credentialsPath, err))
	}
	credentials, err = google.CredentialsFromJSON(r.ctx, data)
	if err != nil {
		return "", errors.New(fmt.Sprintf("Error getting credentials from data in %s: %v", credentialsPath, err))
	}
	if credentials.ProjectID == "" {
		return "", errors.New("no project found in either application default credentials or json file")
	}
	fmt.Printf("Project ID found from supplied credentials: %s\n", credentials.ProjectID)
	return credentials.ProjectID, nil
}

func (r *resourceManager) GetOrgIdFromProjectId(projectId string) error {
	thisProjectAncestry, err := r.GetAncestryForProject(projectId)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to get org for project %s: %v", projectId, err))
	}
	r.orgId = thisProjectAncestry[len(thisProjectAncestry)-1].ResourceId.Id
	fmt.Printf("OrgId of %s found from Project ID %s\n", r.orgId, projectId)
	return nil
}

func (r *resourceManager) OrganizationsList() ([]*v1beta1.Organization, error) {
	orgListReq := r.v1.Organizations.List()
	orgs := make([]*v1beta1.Organization, 0)
	if err := orgListReq.Pages(r.ctx, func(page *v1beta1.ListOrganizationsResponse) error {
		for _, org := range page.Organizations {
			orgs = append(orgs, org)
		}
		return nil
	}); err != nil {
		return []*v1beta1.Organization{}, err
	}
	return orgs, nil
}

type Project struct {
	Name      string
	ProjectId string
}

func (r *resourceManager) ProjectsList() ([]*Project, error) {
	return r.ProjectsListByFilter(fmt.Sprintf("parent.type:organization parent.id:%s", r.orgId))
}

func (r *resourceManager) ProjectsListByFilter(filter string) ([]*Project, error) {
	projects := make([]*Project, 0)
	pListReq := r.v1.Projects.List()
	if filter != "" {
		pListReq.Filter(filter)
	}
	if err := pListReq.Pages(r.ctx, func(page *v1beta1.ListProjectsResponse) error {
		for _, p := range page.Projects {
			projects = append(projects,
				&Project{
					Name:      p.Name,
					ProjectId: p.ProjectId,
				},
			)
		}
		return nil
	}); err != nil {
		return []*Project{}, err
	}
	return projects, nil
}

func (r *resourceManager) FoldersList(parent string) ([]*v2beta1.Folder, error) {
	folders := make([]*v2beta1.Folder, 0)
	fListReq := r.v2.Folders.List()
	if parent != "" {
		fListReq.Parent(parent)
	}
	if err := fListReq.Pages(r.ctx, func(page *v2beta1.ListFoldersResponse) error {
		for _, f := range page.Folders {
			folders = append(folders, f)
		}
		return nil
	}); err != nil {
		return []*v2beta1.Folder{}, err
	}
	return folders, nil
}

type Ancestor struct {
	ResourceId *ResourceId `json:"resourceId,omitempty"`
}

type ResourceId struct {
	Id   string `json:"id,omitempty"`
	Type string `json:"type,omitempty"`
}

func (r *resourceManager) GetAncestryForProject(projectId string) ([]*Ancestor, error) {
	gacall := r.v1.Projects.GetAncestry(projectId, &v1beta1.GetAncestryRequest{})
	garesp, err := gacall.Context(r.ctx).Do()
	if err != nil {
		return []*Ancestor{}, err
	}
	return convertAncestors(garesp.Ancestor), nil
}

func convertAncestors(ancestors interface{}) []*Ancestor {
	var results []*Ancestor
	if v1, ok := ancestors.([]*v1beta1.Ancestor); ok {
		results = make([]*Ancestor, len(v1))
		for i, a := range v1 {
			results[i] = &Ancestor{&ResourceId{a.ResourceId.Id, a.ResourceId.Type}}
		}
	}
	return results
}

type Policy struct {
	Bindings []*Binding `json:"bindings,omitempty"`
	Etag     string     `json:"etag,omitempty"`
}

func (p *Policy) convertV1(policy *v1beta1.Policy) {
	p.Etag = policy.Etag
	p.convertBindingsV1(policy.Bindings)
}

func (p *Policy) convertV2(policy *v2beta1.Policy) {
	p.Etag = policy.Etag
	p.convertBindingsV2(policy.Bindings)
}

func (p *Policy) convertBindingsV1(bindings []*v1beta1.Binding) {
	p.Bindings = make([]*Binding, len(bindings))
	for i, b := range bindings {
		p.Bindings[i] = &Binding{}
		p.Bindings[i].convertV1(b)
	}
}
func (p *Policy) convertBindingsV2(bindings []*v2beta1.Binding) {
	p.Bindings = make([]*Binding, len(bindings))
	for i, b := range bindings {
		p.Bindings[i] = &Binding{}
		p.Bindings[i].convertV2(b)
	}
}

type Binding struct {
	Condition *Expr    `json:"condition,omitempty"`
	Members   []string `json:"members,omitempty"`
	Role      string   `json:"role,omitempty"`
}

func (b *Binding) convertV1(binding *v1beta1.Binding) {
	b.Members = binding.Members
	b.Role = binding.Role
	if b.Condition != nil {
		b.Condition = &Expr{}
		b.Condition.convertV1(binding.Condition)
	}
}
func (b *Binding) convertV2(binding *v2beta1.Binding) {
	b.Members = binding.Members
	b.Role = binding.Role
	if b.Condition != nil {
		b.Condition = &Expr{}
		b.Condition.convertV2(binding.Condition)
	}
}

type Expr struct {
	Description string `json:"description,omitempty"`
	Expression  string `json:"expression,omitempty"`
	Location    string `json:"location,omitempty"`
	Title       string `json:"title,omitempty"`
}

func (e *Expr) convertV1(expr *v1beta1.Expr) {
	e.Description = expr.Description
	e.Expression = expr.Expression
	e.Location = expr.Location
	e.Title = expr.Title
}
func (e *Expr) convertV2(expr *v2beta1.Expr) {
	e.Description = expr.Description
	e.Expression = expr.Expression
	e.Location = expr.Location
	e.Title = expr.Title
}

func (r *resourceManager) GetIamPolicyForProject(projectId string) (*Policy, error) {

	policy := &Policy{}
	gpcall := r.v1.Projects.GetIamPolicy(fmt.Sprintf("%s", projectId), &v1beta1.GetIamPolicyRequest{})
	policyResponse, err := gpcall.Context(r.ctx).Do()
	if err != nil {
		return policy, err
	}
	policy.convertV1(policyResponse)
	return policy, nil
}

func (r *resourceManager) GetIamPolicyForOrganization() (*Policy, error) {
	policy := &Policy{}
	gpcall := r.v1.Organizations.GetIamPolicy(fmt.Sprintf("organizations/%s", r.orgId), &v1beta1.GetIamPolicyRequest{})
	policyResponse, err := gpcall.Context(r.ctx).Do()
	if err != nil {
		return policy, err
	}
	policy.convertV1(policyResponse)
	return policy, nil
}

func (r *resourceManager) GetIamPolicyForFolder(folderId string) (*Policy, error) {

	policy := &Policy{}
	gpcall := r.v2.Folders.GetIamPolicy(fmt.Sprintf("%s", folderId), &v2beta1.GetIamPolicyRequest{})
	policyResponse, err := gpcall.Context(r.ctx).Do()
	if err != nil {
		return policy, err
	}
	policy.convertV2(policyResponse)
	return policy, nil
}

func addBindings(bindings []*Binding, rows *[]*Row, resource string, resType string) {
	for _, b := range bindings {
		for _, m := range b.Members {
			row := &Row{
				Resource: resource,
				Type:     resType,
				Role:     b.Role,
				Member:   m,
			}
			*rows = append(*rows, row)
		}
	}
}

func (r *resourceManager) GetFolderPolicyRows() (*[]*Row, error) {
	var rows []*Row
	rows = make([]*Row, 0)
	folders, err := r.FoldersList(fmt.Sprintf("organizations/%s", r.orgId))
	if err != nil {
		return &rows, err
	}
	for _, f := range folders {
		policy, err := r.GetIamPolicyForFolder(f.Name)
		if err != nil {
			logerr.Printf("Unable to get more info on folder %s: %v\n", f.Name, err)
			return &rows, err
		}
		addBindings(policy.Bindings, &rows, f.Name, "folder")
	}
	return &rows, nil
}

func (r *resourceManager) GetProjectPolicyRows() (*[]*Row, error) {
	var rows []*Row
	rows = make([]*Row, 0)

	projects, err := r.ProjectsList()
	if err != nil {
		return &rows, err
	}
	for _, p := range projects {
		policy, err := r.GetIamPolicyForProject(p.ProjectId)
		if err != nil {
			logerr.Printf("Unable to get more info on project %s: %v\n", p.Name, err)
			return &rows, err
		}
		addBindings(policy.Bindings, &rows, p.Name, "project")
	}
	return &rows, nil
}

func (r *resourceManager) GetOrgPolicyRows() (*[]*Row, error) {
	var rows []*Row
	rows = make([]*Row, 0)

	orgPolicy, err := r.GetIamPolicyForOrganization()
	if err != nil {
		return &rows, err
	}
	addBindings(orgPolicy.Bindings, &rows, r.orgId, "organization")

	return &rows, nil
}

func (r *resourceManager) GetAllPolicyRows() (*[]*Row, error) {
	allRows := make([]*Row, 0)
	newRows, err := r.GetOrgPolicyRows()
	if err != nil {
		return nil, err
	}
	allRows = append(allRows, *newRows...)

	newRows, err = r.GetFolderPolicyRows()
	if err != nil {
		return nil, err
	}
	allRows = append(allRows, *newRows...)

	newRows, err = r.GetProjectPolicyRows()
	if err != nil {
		return nil, err
	}
	allRows = append(allRows, *newRows...)
	return &allRows, nil
}
