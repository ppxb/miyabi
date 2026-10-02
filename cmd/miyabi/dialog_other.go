//go:build !windows

package main

// messageBox has no portable equivalent; every other platform has a console or
// a service log to report the failure.
func messageBox(string, string) {}
