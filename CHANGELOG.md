# Changelog

## Unreleased

- Add Russian and Arabic README, installation, compatibility and device-testing guides.
- Add `--lang ru` / `--lang ar` installer messages and `ru` / `ar` uninstall messages.
- Link all four languages; keep published rc.1 archives unchanged and clearly
  distinguish their English/Persian installer from the next source build.

## 0.2.0-rc.1 — community testing candidate

- Add US-English and Persian entry points and full installation/testing guides.
- Expose the original corresponding core source with provenance and GPL license.
- Add five CPU build targets and source-inclusive archives with checksums.
- Add preflight-only mode, explicit experimental opt-in, executable validation,
  staged initial configuration and restoration attempts for managed program files.
- Keep credentials private by default instead of printing them at install completion.
- Add local allowlisted diagnostics, bilingual issue forms, host checks and CI builds.
- Preserve exact reference-device runtime evidence without claiming the new installer
  or other firmware/device combinations have passed hardware testing.

## 0.1.0

- Original Google WiFi (Gale), OpenWrt 25.12.1 ARMv7 integration archive.
- procd service, LuCI dashboard shortcut and PassWall2 setup instructions.
- Runtime components reported working, including after reboot; integrated installer
  not verified on a real device.

