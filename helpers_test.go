package main

import (
	"testing"
)

func TestDomainFromZone(t *testing.T) {
	cases := []struct {
		zone string
		want string
	}{
		{"home.irwin.earth.", "irwin.earth"},
		{"irwin.earth.", "irwin.earth"},
		{"irwin.earth", "irwin.earth"},
		{"sub.home.irwin.earth.", "irwin.earth"},
		{"example.com.", "example.com"},
	}
	for _, c := range cases {
		got := domainFromZone(c.zone)
		if got != c.want {
			t.Errorf("domainFromZone(%q) = %q, want %q", c.zone, got, c.want)
		}
	}
}

func TestExtractHostAndDomain(t *testing.T) {
	cases := []struct {
		fqdn       string
		zone       string
		wantHost   string
		wantDomain string
	}{
		{
			fqdn:       "_acme-challenge.antmound.home.irwin.earth.",
			zone:       "home.irwin.earth.",
			wantHost:   "_acme-challenge.antmound.home",
			wantDomain: "irwin.earth",
		},
		{
			fqdn:       "_acme-challenge.irwin.earth.",
			zone:       "irwin.earth.",
			wantHost:   "_acme-challenge",
			wantDomain: "irwin.earth",
		},
		{
			// wildcard cert: cert-manager presents _acme-challenge at the zone apex
			fqdn:       "_acme-challenge.home.irwin.earth.",
			zone:       "irwin.earth.",
			wantHost:   "_acme-challenge.home",
			wantDomain: "irwin.earth",
		},
		{
			// no trailing dots
			fqdn:       "_acme-challenge.antmound.home.irwin.earth",
			zone:       "home.irwin.earth",
			wantHost:   "_acme-challenge.antmound.home",
			wantDomain: "irwin.earth",
		},
		{
			fqdn:       "_acme-challenge.example.com.",
			zone:       "example.com.",
			wantHost:   "_acme-challenge",
			wantDomain: "example.com",
		},
	}
	for _, c := range cases {
		host, domain := extractHostAndDomain(c.fqdn, c.zone)
		if host != c.wantHost || domain != c.wantDomain {
			t.Errorf("extractHostAndDomain(%q, %q) = (%q, %q), want (%q, %q)",
				c.fqdn, c.zone, host, domain, c.wantHost, c.wantDomain)
		}
	}
}
