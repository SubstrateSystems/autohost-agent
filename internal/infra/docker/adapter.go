// Package docker provides infrastructure operations for Docker:
// installation, compose lifecycle, networking, and user-group management.
//
// All functions are package-level and stateless. They delegate to the
// Docker CLI / system tools available on the host.
package docker
