package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-sky/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Sky as the
// target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "sky:read", Name: "See your night sky", Description: "Read tonight's sky at your places (how dark and clear it will be, the Moon, which planets are up and when, the best sights), the coming nights, sky events like eclipses and meteor showers, where anything in the sky is, and your observing log and want-to-see list — for your day, your calendar, dashboards and Pocket AI."},
	{Scope: "sky:write", Name: "Add to your observing log", Description: "Log a night under the sky with what you saw, add something to your want-to-see list, and change or remove only the entries that app added itself."},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(overview|summary|tonight|nights|bodies|darkness|events|catalog|search|objects/search|observations|targets|progress|locations|day/\d{4}-\d{2}-\d{2})$`), "sky:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(observations|locations)/\d+$`), "sky:read"},
	{"GET", regexp.MustCompile(`^/api/v1/objects/[a-z]+(:[A-Za-z0-9-]+)?$`), "sky:read"},
	{"POST", regexp.MustCompile(`^/api/v1/(observations|targets)$`), "sky:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/observations/\d+$`), "sky:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "sky" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
