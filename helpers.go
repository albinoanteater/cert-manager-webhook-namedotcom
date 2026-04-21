package main

import "strings"

// domainFromZone returns the 2-label registered domain from a DNS zone name.
// "home.irwin.earth." → "irwin.earth"
// "irwin.earth."     → "irwin.earth"
func domainFromZone(zone string) string {
	zone = strings.TrimSuffix(zone, ".")
	parts := strings.Split(zone, ".")
	if len(parts) < 2 {
		return zone
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// extractHostAndDomain returns the (host, domain) values needed for a name.com DNS record call.
// fqdn and zone may or may not have a trailing dot.
//
// Example:
//
//	fqdn="_acme-challenge.antmound.home.irwin.earth." zone="home.irwin.earth."
//	→ host="_acme-challenge.antmound.home", domain="irwin.earth"
func extractHostAndDomain(fqdn, zone string) (host, domain string) {
	fqdn = strings.TrimSuffix(fqdn, ".")
	domain = domainFromZone(zone)
	host = strings.TrimSuffix(fqdn, "."+domain)
	return host, domain
}
