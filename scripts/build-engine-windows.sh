#!/usr/bin/env bash
# Cross-build the FDPI engine (winws2.exe) for Windows from Linux.
# winws2 is the WinDivert-based Windows frontend of zapret2's nfq2, built with
# mingw-w64 instead of upstream's cygwin via scripts/windows/mingw-compat shims.
set -euo pipefail
cd -- "$(dirname -- "$0")/.."

commit=9b9b929f837978173d787a5773a47a91d90f7f81
MIRROR="$PWD/scripts/windows/mingw-compat"
DEPS=/tmp/fdpi-win-deps

CC=${CC:-x86_64-w64-mingw32-gcc}
AR=${AR:-x86_64-w64-mingw32-ar}
command -v "$CC" >/dev/null || { echo "Нужен mingw-w64 (x86_64-w64-mingw32-gcc)" >&2; exit 1; }
command -v git >/dev/null || { echo "Нужен git" >&2; exit 1; }

mkdir -p build "$DEPS"

# --- zapret2 sources at the pinned commit ---
if [[ ! -d build/zapret2/.git ]]; then
    git clone --no-checkout https://github.com/bol-van/zapret2.git build/zapret2
fi
git -C build/zapret2 fetch origin "$commit"
git -C build/zapret2 checkout --detach "$commit"

# --- apply mingw patch + rename the remaining cygwin-only guards ---
patched=build/zapret2/.fdpi-mingw-patched
if [[ ! -f "$patched" ]]; then
    git -C build/zapret2 apply "$PWD/scripts/windows/patches/zapret2-mingw.patch"
    for f in build/zapret2/nfq2/*.c build/zapret2/nfq2/*.h build/zapret2/nfq2/crypto/*.c build/zapret2/nfq2/crypto/*.h; do
        case "$f" in
            */helpers.c|*/lua.c|*/sec.c) continue ;; # already renamed by the patch
        esac
        sed -i 's/__CYGWIN__/ZAPRET_WIN/g' "$f"
    done
    touch "$patched"
fi

# --- Lua 5.4 static library for mingw ---
if [[ ! -f "$DEPS/liblua-mingw.a" ]]; then
    if [[ ! -d "$DEPS/lua-5.4.8/src" ]]; then
        curl -fsSL https://www.lua.org/ftp/lua-5.4.8.tar.gz | tar xz -C "$DEPS"
    fi
    for f in lapi lauxlib lbaselib lcode lcorolib lctype ldblib ldebug ldo ldump lfunc lgc \
             linit liolib llex lmathlib lmem loadlib lobject lopcodes loslib lparser lstate \
             lstring lstrlib ltable ltablib ltm lundump lutf8lib lvm lzio; do
        "$CC" -O2 -c "$DEPS/lua-5.4.8/src/$f.c" -o "$DEPS/lua-5.4.8/src/$f.o"
    done
    "$AR" rcs "$DEPS/liblua-mingw.a" "$DEPS"/lua-5.4.8/src/l*.o
fi

# --- zlib static library for mingw ---
if [[ ! -f "$DEPS/libz.a" ]]; then
    if [[ ! -d "$DEPS/zlib-1.3.1" ]]; then
        curl -fsSL https://zlib.net/fossils/zlib-1.3.1.tar.gz | tar xz -C "$DEPS"
    fi
    make -s -C "$DEPS/zlib-1.3.1" -f win32/Makefile.gcc PREFIX=${CC%-gcc}- libz.a
    cp "$DEPS/zlib-1.3.1/libz.a" "$DEPS/libz.a"
fi

# --- compile winws2 ---
NFQ=build/zapret2/nfq2
CF="-std=gnu99 -O2 -DZAPRET_WIN -D_WIN32_WINNT=0x0600"
CF+=" -include $MIRROR/include/zcompat_extra.h"
CF+=" -I$MIRROR/include -I$NFQ/windows -I$DEPS/lua-5.4.8/src -I$DEPS/zlib-1.3.1"
OBJ=build/windows-obj
mkdir -p "$OBJ"
srcs=("$NFQ"/*.c "$NFQ"/crypto/*.c "$MIRROR"/compat.c)
for f in "${srcs[@]}"; do
    o="$OBJ/$(echo "${f#$NFQ/}" | tr '/' '_').o"
    "$CC" $CF -c "$f" -o "$o"
done
"$CC" -static -Wl,--gc-sections -o build/winws2.exe "$OBJ"/*.o \
    "$DEPS/liblua-mingw.a" "$DEPS/libz.a" \
    -L"$NFQ/windows/windivert" -lwindivert64 \
    -lws2_32 -liphlpapi -lole32 -loleaut32 -luuid -lntdll -lwlanapi -ladvapi32 -lshlwapi

mkdir -p build/windows/lua
cp build/winws2.exe build/windows/
install -m644 build/zapret2/lua/zapret-lib.lua build/zapret2/lua/zapret-antidpi.lua build/windows/lua/
install -m644 build/zapret2/docs/LICENSE.txt build/windows/ZAPRET-LICENSE
printf '%s\n' "$commit" > build/windows/engine.commit
cp third_party/windivert/x64/WinDivert.dll build/windows/
( cd build/windows && { wine ./winws2.exe --version || ./winws2.exe --version; } )
echo "Движок собран: build/windows/winws2.exe"
