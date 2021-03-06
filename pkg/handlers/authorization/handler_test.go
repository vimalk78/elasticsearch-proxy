package authorization

import (
	"fmt"
	"net/http"

	"github.com/bluele/gcache"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	authenticationapi "k8s.io/api/authentication/v1"

	"github.com/openshift/elasticsearch-proxy/pkg/clients"
	"github.com/openshift/elasticsearch-proxy/pkg/config"
	"github.com/openshift/elasticsearch-proxy/pkg/handlers"
	"github.com/openshift/elasticsearch-proxy/pkg/handlers/clusterlogging/types"
)

var _ = Describe("Process", func() {

	var (
		err        error
		req        *http.Request
		context    = &handlers.RequestContext{}
		handler    *authorizationHandler
		cacheEntry *rolesProjects
	)

	BeforeEach(func() {
		req, _ = http.NewRequest("post", "https://someplace", nil)
		req.Header.Set("X-OCP-NS", "deleteme")
		req.Header.Set("X-Forwarded-Roles", "deleteme")
		handler = &authorizationHandler{
			config: &config.Options{
				AuthBackEndRoles: map[string]config.BackendRoleConfig{
					"roleA": config.BackendRoleConfig{},
					"roleB": config.BackendRoleConfig{},
				},
			},
			fnSubjectExtractor: func(req *http.Request) string {
				return "CN=foo,OU=org-unit,O=org"
			},
		}
	})

	Context("when certs are provided", func() {
		Context("without bearer token and does not error", func() {
			BeforeEach(func() {
				req, err = handler.Process(req, context)
				Expect(err).To(BeNil())
			})
			It("should pass the subject as the user", func() {
				Expect(req.Header.Get("X-Forwarded-User")).To(Equal("CN=foo,OU=org-unit,O=org"))
			})
			It("should sanitize the headers", func() {
				Expect(req.Header.Get("Authorization")).To(BeEmpty())
				Expect(req.Header.Get("X-Forwarded-Roles")).To(BeEmpty())
				Expect(req.Header.Get("X-OCP-NS")).To(BeEmpty())
				Expect(req.Header.Get("X-OCP-NSUID")).To(BeEmpty())
			})
		})
		Context("with empty bearer token and does not error", func() {
			It("should pass the subject as the user", func() {
				req.Header.Set("Authorization", "Bearer  ")
				req, err = handler.Process(req, context)
				Expect(err).To(BeNil())
				Expect(req.Header.Get("X-Forwarded-User")).To(Equal("CN=foo,OU=org-unit,O=org"))
			})
		})
		Context("and it returns an empty subject", func() {
			It("should error", func() {
				handler.fnSubjectExtractor = func(req *http.Request) string {
					return "  "
				}
				_, err = handler.Process(req, context)
				Expect(err).To(Not(BeNil()))
			})
		})
	})

	Context("when a bearer token without certs is provided and it does not error", func() {

		BeforeEach(func() {
			req.Header.Set("Authorization", "Bearer somebearertoken")
			cacheEntry = &rolesProjects{
				review: &clients.TokenReview{
					&authenticationapi.TokenReview{
						Status: authenticationapi.TokenReviewStatus{
							User: authenticationapi.UserInfo{
								Username: "myname",
							},
						},
					},
				},
				roles: map[string]struct{}{
					"roleA": struct{}{},
					"roleB": struct{}{},
				},
				projects: []types.Project{
					types.Project{
						Name: "projecta",
						UUID: "projectauuid",
					},
					types.Project{
						Name: "projectb",
						UUID: "projectbuuid",
					},
				},
			}
			handler.cache = &rolesService{
				cache: gcache.New(1).
					LRU().
					LoaderFunc(func(key interface{}) (interface{}, error) {
						return cacheEntry, nil
					}).
					Build(),
			}
			req, err = handler.Process(req, context)
			Expect(err).To(BeNil())
		})

		It("should add forward user to the request", func() {
			Expect(req.Header.Get("X-Forwarded-User")).To(Equal("myname"))
		})
		It("should add role headers to the request", func() {
			entries, ok := req.Header["X-Forwarded-Roles"]
			Expect(ok).To(BeTrue(), fmt.Sprintf("Expected a user's roles to be added to be proxy headers: %v", req.Header))
			Expect(entries).To(Equal([]string{"roleA,roleB"}))
		})
		It("should add a user's projects to the request", func() {
			entries, ok := req.Header["X-Ocp-Ns"]
			Expect(ok).To(BeTrue(), fmt.Sprintf("Expected a user's projects to be added to be proxy headers: %v", req.Header))
			Expect(entries).To(Equal([]string{"projecta,projectb"}))
		})
		It("should add a user's project uids to the request", func() {
			entries, ok := req.Header["X-Ocp-Nsuid"]
			Expect(ok).To(BeTrue(), fmt.Sprintf("Expected a project uids to be added to be proxy headers: %v", req.Header))
			Expect(entries).To(Equal([]string{"projectauuid,projectbuuid"}))
		})
	})

})
