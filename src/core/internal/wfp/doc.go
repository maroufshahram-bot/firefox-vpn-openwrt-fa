// Package wfp keeps DNS from leaving the computer outside the TUN adapter
// while the TUN mode runs on Windows, with Windows Filtering Platform
// filters: Windows asks every network's DNS servers at once (the router's
// too, which is on the local network and so outside the TUN's routes), and
// on a filtered network those answers are wrong, and give away the names.
//
// sing-tun's strict_route does the same but allows only its own process;
// the core is another process and needs the physical network's DNS for its
// direct routes and its own fallback, so the TUN service sets these
// filters itself, for the programs it names.
//
// The filters live in a dynamic session: Windows removes them when the
// session closes, also when the service exits or crashes.
package wfp
