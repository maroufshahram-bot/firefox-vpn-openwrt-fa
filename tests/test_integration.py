"""Host fixtures: no router, Mozilla account or Internet access is used."""
import hashlib
import importlib.util
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('packager', ROOT / 'scripts/package.py')
packager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packager)


class PlatformTests(unittest.TestCase):
    def check(self, expression):
        return subprocess.run(['sh', '-c', '. "$1"; ' + expression, '_', str(ROOT / 'packaging/platform.sh')]).returncode

    def test_architecture_gate(self):
        for target, machine in [('armv7','armv7l'), ('arm64','aarch64'), ('amd64','x86_64'), ('mips','mips'), ('mipsle','mips')]:
            self.assertEqual(self.check(f'ffvpn_arch_matches {target} {machine}'), 0)
        for target, machine in [('armv7','aarch64'), ('amd64','i686'), ('arm64','armv6l'), ('mips','x86_64')]:
            self.assertNotEqual(self.check(f'ffvpn_arch_matches {target} {machine}'), 0)

    def test_reference_is_exact(self):
        self.assertEqual(self.check("ffvpn_reference_platform OpenWrt 25.12.1 ipq40xx/chromium 'Google WiFi (Gale)' armv7l"), 0)
        for field in ['OpenWrt', '25.12.1', 'ipq40xx/chromium', 'Google WiFi (Gale)', 'armv7l']:
            args = ['OpenWrt', '25.12.1', 'ipq40xx/chromium', 'Google WiFi (Gale)', 'armv7l']
            args[args.index(field)] = 'different'
            import shlex
            self.assertNotEqual(self.check('ffvpn_reference_platform ' + ' '.join(map(shlex.quote,args))), 0)


class PackagingTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.original_root = packager.ROOT
        self.addCleanup(setattr, packager, 'ROOT', self.original_root)
        packager.ROOT = Path(self.temp.name)
        (packager.ROOT / 'VERSION').write_text('0.2.0-rc.1\n')
        self.stage = packager.ROOT / 'build/armv7'
        (self.stage / 'bin').mkdir(parents=True)
        header = bytearray(64)
        header[:6] = b'\x7fELF\x01\x01'
        header[18:20] = (40).to_bytes(2, 'little')
        self.binary = self.stage / 'bin/firefox-vpn-core'
        self.binary.write_bytes(header)
        (self.stage / 'TARGET').write_text('armv7\n')
        self.checksum()

    def checksum(self):
        (self.stage / 'SHA256SUMS').write_text(hashlib.sha256(self.binary.read_bytes()).hexdigest() + '  bin/firefox-vpn-core\n')

    def test_repeatable_package_and_metadata(self):
        packager.package('armv7')
        path = next((packager.ROOT / 'dist').glob('*.tar.gz'))
        before = path.read_bytes()
        os.utime(self.binary, (12345, 12345))
        packager.package('armv7')
        self.assertEqual(before, path.read_bytes())
        with tarfile.open(path) as archive:
            for member in archive.getmembers():
                self.assertEqual(member.uid, 0)
                self.assertEqual(member.mtime, 0)
                self.assertTrue(member.name.startswith('firefox-vpn-openwrt'))

    def test_checksum_rejects_modified_binary(self):
        self.binary.write_bytes(self.binary.read_bytes() + b'changed')
        with self.assertRaisesRegex(SystemExit, 'checksum'):
            packager.package('armv7')

    def test_wrong_endianness_or_cpu_is_rejected(self):
        header = bytearray(self.binary.read_bytes()); header[5] = 2
        self.binary.write_bytes(header); self.checksum()
        with self.assertRaisesRegex(SystemExit, 'ELF CPU'):
            packager.package('armv7')


@unittest.skipUnless(os.name == 'posix', 'requires POSIX shell fixtures')
class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for folder in ['bin','sbin','tmp','dev','etc/init.d','usr','www/luci-static/resources','root','pkg/bin']:
            (self.root / folder).mkdir(parents=True, exist_ok=True)
        (self.root / 'dev/null').touch()
        for path in (ROOT / 'packaging').glob('*.sh'):
            self.file('pkg/' + path.name, path.read_text())
        self.file('etc/openwrt_release', "DISTRIB_ID='OpenWrt'\nDISTRIB_RELEASE='25.12.1'\nDISTRIB_TARGET='ipq40xx/chromium'\n")
        self.file('www/luci-static/resources/view.js', '')
        self.script('sbin/procd', 'exit 0')
        self.script('bin/id', 'echo 0')
        self.script('bin/uname', 'echo armv7l')
        self.script('bin/df', "echo 'Filesystem 1024-blocks Used Available Capacity Mounted'; echo 'test 1000000 0 1000000 0% /'")
        self.script('bin/ubus', 'case "$*" in *system*) echo board;; *) echo lan;; esac')
        self.script('bin/jsonfilter', '''case "$*" in
*dashboard.listen*) echo '192.0.2.1:10820';;
*proxy.listen*) echo '127.0.0.1:10810';;
*) read -r input; case "$input" in board) echo 'Google WiFi (Gale)';; lan) echo '192.0.2.1';; esac;;
esac''')
        self.script('etc/rc.common', '''case "$2" in
start) [ ! -f /FAIL_START ] || exit 1; echo yes > /tmp/service-running;;
stop) rm -f /tmp/service-running;;
running) [ -f /tmp/service-running ];;
enable) echo yes > /tmp/service-enabled;;
disable) rm -f /tmp/service-enabled;;
esac''')
        self.script('pkg/bin/firefox-vpn-core', '''case "$1" in
-version) [ ! -f /FAIL_VERSION ];;
-setup-server)
    [ ! -f /FAIL_SETUP ] || exit 1
    mkdir -p "$4"
    echo '{"dashboard":{"listen":"0.0.0.0:10820"},"proxy":{"listen":"0.0.0.0:10810"}}' > "$4/config.json"
    echo PRIVATE_TEST_PASSWORD;;
esac''')
        self.file('pkg/TARGET', 'armv7\n')
        self.file('pkg/PACKAGE_VERSION', '0.2.0-rc.1\n')
        digest = hashlib.sha256((self.root / 'pkg/bin/firefox-vpn-core').read_bytes()).hexdigest()
        self.file('pkg/SHA256SUMS', digest + '  bin/firefox-vpn-core\n')

    def file(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        # Redirect integration filesystem paths into an isolated fixture tree.
        # Production scripts are not given a filesystem-override option.
        text = re.sub(r"/(?:etc|usr|www|root|tmp|sbin)(?=[/\s\"';]|$)|/FAIL_[A-Z_]+", lambda m: str(self.root) + m.group(), text)
        path.write_text(text)

    def script(self, name, text):
        self.file(name, '#!/bin/sh\n' + text + '\n')
        (self.root / name).chmod(0o755)

    def run_install(self, *args):
        return subprocess.run(['/bin/sh', str(self.root / 'pkg/install.sh'), *args], env={'PATH':str(self.root / 'bin')+':/usr/bin:/bin','LC_ALL':'C'}, capture_output=True, text=True)

    def test_preflight_does_not_install(self):
        result = self.run_install('--check')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse((self.root / 'etc/firefox-vpn').exists())
        self.assertFalse((self.root / 'etc/init.d/firefox-vpn').exists())

    def test_localized_preflight_and_failures(self):
        messages = {
            'en': ('Preflight passed', 'Wrong CPU package'),
            'fa': ('بررسی پیش‌نیازها موفق بود', 'بسته با معماری پردازنده سازگار نیست'),
            'ru': ('Предварительная проверка пройдена', 'Неверный пакет CPU'),
            'ar': ('نجح الفحص المسبق', 'حزمة المعالج غير مناسبة'),
        }
        for language, (success, failure) in messages.items():
            with self.subTest(language=language):
                self.file('pkg/TARGET', 'armv7\n')
                result = self.run_install('--lang', language, '--check')
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(success, result.stdout)
                self.assertFalse((self.root / 'etc/firefox-vpn').exists())
                self.file('pkg/TARGET', 'arm64\n')
                result = self.run_install('--lang', language, '--check')
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(failure, result.stderr)
                self.assertFalse((self.root / 'etc/init.d/firefox-vpn').exists())
        self.assertEqual(self.run_install('--lang', 'invalid', '--check').returncode, 2)

    def test_russian_arabic_install_and_uninstall_keep_accounts_private(self):
        for language, installed, removed in [('ru', 'Установлено.', 'Удалено.'),
                                               ('ar', 'تم التثبيت.', 'تمت الإزالة.')]:
            with self.subTest(language=language):
                result = self.run_install('--lang', language)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(installed, result.stdout)
                self.assertNotIn('PRIVATE_TEST_PASSWORD', result.stdout + result.stderr)
                self.assertEqual((self.root / 'etc/firefox-vpn/initial-login.txt').read_text().strip(), 'PRIVATE_TEST_PASSWORD')
                result = subprocess.run(['/bin/sh', str(self.root / 'pkg/uninstall.sh'), language],
                    env={'PATH':str(self.root / 'bin')+':/usr/bin:/bin'}, capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(removed, result.stdout)
                self.assertTrue((self.root / 'etc/firefox-vpn/config.json').exists())
                self.assertFalse((self.root / 'etc/init.d/firefox-vpn').exists())

    def test_experimental_requires_opt_in(self):
        self.file('etc/openwrt_release', "DISTRIB_ID='ImmortalWrt'\nDISTRIB_RELEASE='24.10'\n")
        self.assertNotEqual(self.run_install('--check').returncode, 0)
        self.assertEqual(self.run_install('--experimental','--check').returncode, 0)

    def test_unsupported_family_cannot_be_forced(self):
        self.file('etc/openwrt_release', "DISTRIB_ID='RouterOS'\n")
        self.assertNotEqual(self.run_install('--experimental','--check').returncode, 0)

    def test_wrong_cpu_and_corruption_rejected(self):
        self.file('pkg/TARGET', 'arm64\n')
        self.assertNotEqual(self.run_install('--check').returncode, 0)
        self.file('pkg/TARGET', 'armv7\n')
        self.file('pkg/SHA256SUMS', 'BAD  bin/firefox-vpn-core\n')
        self.assertNotEqual(self.run_install('--check').returncode, 0)

    def test_failed_binary_probe_rejected(self):
        self.file('FAIL_VERSION','')
        self.assertNotEqual(self.run_install('--check').returncode, 0)

    def test_legacy_install_is_preserved(self):
        self.file('root/firefox-openwrt-armv7-test1/data/config.json','legacy')
        self.assertNotEqual(self.run_install().returncode, 0)
        self.assertFalse((self.root / 'etc/init.d/firefox-vpn').exists())

    def test_setup_failure_leaves_existing_program_untouched(self):
        self.file('FAIL_SETUP','')
        self.file('usr/lib/firefox-vpn/firefox-vpn-core','old-binary')
        self.assertNotEqual(self.run_install().returncode, 0)
        self.assertEqual((self.root / 'usr/lib/firefox-vpn/firefox-vpn-core').read_text(),'old-binary')

    def test_fresh_install_keeps_credentials_out_of_output(self):
        result = self.run_install('--lang','fa')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('PRIVATE_TEST_PASSWORD', result.stdout + result.stderr)
        self.assertTrue((self.root / 'tmp/service-enabled').exists())
        self.assertEqual((self.root / 'etc/firefox-vpn/initial-login.txt').read_text().strip(),'PRIVATE_TEST_PASSWORD')
        config = (self.root / 'etc/firefox-vpn/config.json').read_text()
        self.assertIn('192.0.2.1:10820', config)
        self.assertIn('127.0.0.1:10810', config)

    def test_failed_start_restores_managed_files_and_preserves_accounts(self):
        self.file('etc/firefox-vpn/config.json','ACCOUNT_SENTINEL')
        self.file('usr/lib/firefox-vpn/firefox-vpn-core','old-binary')
        self.file('FAIL_START','')
        result = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.root / 'usr/lib/firefox-vpn/firefox-vpn-core').read_text(),'old-binary')
        self.assertEqual((self.root / 'etc/firefox-vpn/config.json').read_text(),'ACCOUNT_SENTINEL')

    def test_reinstall_and_uninstall_preserve_accounts(self):
        self.assertEqual(self.run_install().returncode,0)
        self.file('etc/firefox-vpn/config.json','ACCOUNT_SENTINEL')
        self.assertEqual(self.run_install().returncode,0)
        result = subprocess.run(['/bin/sh',str(self.root / 'pkg/uninstall.sh'),'en'],env={'PATH':str(self.root / 'bin')+':/usr/bin:/bin'},capture_output=True,text=True)
        self.assertEqual(result.returncode,0,result.stderr)
        self.assertEqual((self.root / 'etc/firefox-vpn/config.json').read_text(),'ACCOUNT_SENTINEL')
        self.assertFalse((self.root / 'etc/init.d/firefox-vpn').exists())


if __name__ == '__main__':
    unittest.main()

