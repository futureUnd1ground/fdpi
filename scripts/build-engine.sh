#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")/.."
commit=9b9b929f837978173d787a5773a47a91d90f7f81
command -v git >/dev/null
command -v cc >/dev/null
pkg-config --exists libnetfilter_queue libnfnetlink libmnl libcap || {
    echo 'Установите зависимости сборки: sudo pacman -S --needed base-devel git pkgconf libnetfilter_queue libnfnetlink libmnl libcap' >&2
    exit 1
}
pkg-config --exists zlib || {
    echo 'Не найден API zlib. В CachyOS подходит zlib-ng-compat, в Arch — zlib. Сначала проверьте pacman -T zlib; не заменяйте уже установленный совместимый пакет.' >&2
    exit 1
}
lua_pc=
for candidate in lua lua5.5 lua5.4 lua55 lua54; do
 if pkg-config --exists "$candidate"; then lua_pc=$candidate; break; fi
done
[[ -n $lua_pc ]] || { echo "Нужны development-файлы Lua 5.4/5.5 (на Arch: lua)" >&2; exit 1; }
mkdir -p build
if [[ ! -d build/zapret2/.git ]]; then
    git clone --no-checkout https://github.com/bol-van/zapret2.git build/zapret2
fi
git -C build/zapret2 fetch origin "$commit"
git -C build/zapret2 checkout --detach "$commit"
make -C build/zapret2/nfq2 -j"$(getconf _NPROCESSORS_ONLN)" LUA_JIT=0 LUA_CFLAGS="$(pkg-config --cflags "$lua_pc")" LUA_LIB="$(pkg-config --libs "$lua_pc")"
install -m755 build/zapret2/nfq2/nfqws2 build/nfqws2
mkdir -p build/lua
install -m644 build/zapret2/lua/zapret-lib.lua build/zapret2/lua/zapret-antidpi.lua build/lua/
# Upstream uses docs/LICENSE.txt, keep its exact text beside the binary.
install -m644 build/zapret2/docs/LICENSE.txt build/ZAPRET-LICENSE
printf '%s\n' "$commit" > build/engine.commit
./build/nfqws2 --version
