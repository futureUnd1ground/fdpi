#!/usr/bin/env bash
set -euo pipefail
[[ $EUID == 0 ]] || { echo 'Выполните sudo make uninstall из каталога проекта' >&2; exit 1; }
# Preserve /etc/fdpi and every user configuration. Stop cleanup errors are visible.
if systemctl cat fdpi.service >/dev/null 2>&1; then
    systemctl disable --now fdpi.service
fi
if [[ -x /usr/bin/fdpi ]]; then /usr/bin/fdpi cleanup; fi
rm -f /usr/bin/fdpi /usr/lib/fdpi/nfqws /usr/lib/fdpi/nfqws2 /usr/lib/fdpi/lua/zapret-lib.lua /usr/lib/fdpi/lua/zapret-antidpi.lua /usr/lib/systemd/system/fdpi.service
rm -f /usr/share/licenses/fdpi/LICENSE /usr/share/licenses/fdpi/ZAPRET-LICENSE /usr/share/doc/fdpi/README.md /usr/share/doc/fdpi/INSTALL_RU.md
rmdir --ignore-fail-on-non-empty /usr/lib/fdpi/lua /usr/lib/fdpi /usr/share/licenses/fdpi /usr/share/doc/fdpi
systemctl daemon-reload
printf '%s\n' 'Программа удалена. Настройки /etc/fdpi и ~/.config/fdpi сохранены.'
