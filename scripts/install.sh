#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")/.."
[[ $EUID == 0 ]] || { echo 'Сначала выполните make build и make engine, затем sudo make install.' >&2; exit 1; }
for bin in build/fdpi build/nfqws2; do [[ -f $bin ]] || { echo "Не найден $bin; выполните сборку по инструкции fdpi setup" >&2; exit 1; }; done
[[ -f build/ZAPRET-LICENSE ]] || { echo 'Не найдена лицензия движка; выполните make engine' >&2; exit 1; }
[[ ! -L /etc/fdpi ]] || { echo '/etc/fdpi является символической ссылкой; установка остановлена' >&2; exit 1; }
install -d -m755 /usr/lib/fdpi /usr/share/licenses/fdpi /usr/share/doc/fdpi
install -d -m700 /etc/fdpi
chown root:root /etc/fdpi
exec 9>/etc/fdpi/.operation.lock
flock -n 9 || { echo 'Другая операция FDPI уже выполняется; повторите установку позже.' >&2; exit 1; }
install -m755 build/fdpi /usr/bin/fdpi
install -m755 build/nfqws2 /usr/lib/fdpi/nfqws2
install -d -m755 /usr/lib/fdpi/lua
install -m644 build/lua/zapret-lib.lua build/lua/zapret-antidpi.lua /usr/lib/fdpi/lua/
install -m644 packaging/systemd/fdpi.service /usr/lib/systemd/system/fdpi.service
install -m644 LICENSE /usr/share/licenses/fdpi/LICENSE
install -m644 build/ZAPRET-LICENSE /usr/share/licenses/fdpi/ZAPRET-LICENSE
install -m644 README.md /usr/share/doc/fdpi/README.md
install -m644 docs/INSTALL_RU.md /usr/share/doc/fdpi/INSTALL_RU.md
if [[ ! -e /etc/fdpi/config.json ]]; then
    install -m600 configs/default.json /etc/fdpi/config.json
fi
systemctl daemon-reload
printf '%s\n' 'Файлы установлены. Служба автоматически не запускается и не перезапускается. Продолжите мастер fdpi setup: учёт соединений, выбор сервиса и запуск.'
