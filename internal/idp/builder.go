package idp

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

func buildOAuthManifest(opts IdPOpts) map[string]interface{} {
	var entry map[string]interface{}
	switch opts.Type {
	case IdPGitHub:
		entry = githubIdPEntry(opts)
	case IdPGoogle:
		entry = googleIdPEntry(opts)
	case IdPHTPasswd:
		entry = htpasswdIdPEntry(opts)
	case IdPLDAP:
		entry = ldapIdPEntry(opts)
	case IdPOIDC:
		entry = oidcIdPEntry(opts)
	default:
		entry = githubIdPEntry(opts)
	}

	return map[string]interface{}{
		"apiVersion": "config.openshift.io/v1",
		"kind":       "OAuth",
		"metadata": map[string]interface{}{
			"name": "cluster",
		},
		"spec": map[string]interface{}{
			"identityProviders": []interface{}{entry},
		},
	}
}

func buildSecretManifest(opts IdPOpts) map[string]interface{} {
	secretData := map[string]interface{}{}

	switch opts.Type {
	case IdPGitHub, IdPGoogle, IdPOIDC:
		secretData["clientSecret"] = base64.StdEncoding.EncodeToString([]byte(opts.ClientSecret))
	case IdPHTPasswd:
		secretData["htpasswd"] = base64.StdEncoding.EncodeToString([]byte(buildHTPasswdData(opts.Users)))
	case IdPLDAP:
		secretData["bindPassword"] = base64.StdEncoding.EncodeToString([]byte(opts.BindPassword))
	}

	return map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      secretName(opts.Name),
			"namespace": "openshift-config",
		},
		"type": "Opaque",
		"data": secretData,
	}
}

func secretName(idpName string) string {
	return idpName + "-secret"
}

func githubIdPEntry(opts IdPOpts) map[string]interface{} {
	github := map[string]interface{}{
		"clientID": opts.ClientID,
		"clientSecret": map[string]interface{}{
			"name": secretName(opts.Name),
		},
	}
	if len(opts.Organizations) > 0 {
		orgs := make([]interface{}, len(opts.Organizations))
		for i, o := range opts.Organizations {
			orgs[i] = o
		}
		github["organizations"] = orgs
	}
	return map[string]interface{}{
		"name":          opts.Name,
		"type":          "GitHub",
		"mappingMethod": "claim",
		"github":        github,
	}
}

func googleIdPEntry(opts IdPOpts) map[string]interface{} {
	return map[string]interface{}{
		"name":          opts.Name,
		"type":          "Google",
		"mappingMethod": "claim",
		"google": map[string]interface{}{
			"clientID": opts.ClientID,
			"clientSecret": map[string]interface{}{
				"name": secretName(opts.Name),
			},
		},
	}
}

func htpasswdIdPEntry(opts IdPOpts) map[string]interface{} {
	return map[string]interface{}{
		"name":          opts.Name,
		"type":          "HTPasswd",
		"mappingMethod": "claim",
		"htpasswd": map[string]interface{}{
			"fileData": map[string]interface{}{
				"name": secretName(opts.Name),
			},
		},
	}
}

func ldapIdPEntry(opts IdPOpts) map[string]interface{} {
	return map[string]interface{}{
		"name":          opts.Name,
		"type":          "LDAP",
		"mappingMethod": "claim",
		"ldap": map[string]interface{}{
			"url":      opts.LDAPURL,
			"insecure": opts.Insecure,
			"bindDN":   opts.BindDN,
			"bindPassword": map[string]interface{}{
				"name": secretName(opts.Name),
			},
			"attributes": map[string]interface{}{
				"id":                []interface{}{"dn"},
				"email":             []interface{}{"mail"},
				"name":              []interface{}{"cn"},
				"preferredUsername": []interface{}{"uid"},
			},
		},
	}
}

func oidcIdPEntry(opts IdPOpts) map[string]interface{} {
	return map[string]interface{}{
		"name":          opts.Name,
		"type":          "OpenID",
		"mappingMethod": "claim",
		"openID": map[string]interface{}{
			"clientID": opts.ClientID,
			"clientSecret": map[string]interface{}{
				"name": secretName(opts.Name),
			},
			"issuer": opts.IssuerURL,
			"claims": map[string]interface{}{
				"preferredUsername": []interface{}{"preferred_username"},
				"name":              []interface{}{"name"},
				"email":             []interface{}{"email"},
			},
		},
	}
}

// buildHTPasswdData generates htpasswd-format lines (user:password).
// PoC uses plaintext passwords; production should use bcrypt hashes.
func buildHTPasswdData(users map[string]string) string {
	if len(users) == 0 {
		return ""
	}
	keys := make([]string, 0, len(users))
	for k := range users {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s:%s", k, users[k]))
	}
	return strings.Join(lines, "\n")
}
