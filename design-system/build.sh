#!/usr/bin/env bash
# Сборка design-system/dist/ — standalone-превью компонентов для Claude Design.
#
# Ключевая идея: CSS не дублируется. Каждый превью инлайнит реальные файлы из
# web/static/, перечисленные в директиве @dsCss, поэтому дизайн-система физически
# не может разойтись с продуктовым кодом — достаточно пересобрать.
#
# Формат исходника (design-system/src/cards/<name>.html):
#   <!-- @dsCard group="Foundations" -->   группа в панели Design System (обязательна, первая строка результата)
#   <!-- @dsName Кнопки -->                подпись карточки
#   <!-- @dsSubtitle primary / secondary --> что показано на карточке
#   <!-- @dsCss tokens.css shell.css -->   какие web/static/*.css инлайнить
#   <!-- @dsViewport 900x520 -->           размер карточки в панели
#   <!-- @dsTheme dark -->                 тёмная подложка превью (сайдбар, панели)
#   ...разметка компонента...
set -euo pipefail

ds="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(dirname "$ds")"
static="$root/web/static"
out="$ds/dist"

[ -d "$static" ] || { echo "не найден $static" >&2; exit 1; }

rm -rf "$out"
mkdir -p "$out/components"

tmp="$(mktemp -t dsbuild)"
trap 'rm -f "$tmp"' EXIT

# Рабочее дерево местами в CRLF, поэтому директивы читаем из нормализованной копии.
directive() { sed -n "s/^<!-- @$1 \\(.*\\) -->[[:space:]]*\$/\\1/p" "$tmp" | head -1; }

json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }

meta="$out/cards.json"
printf '[\n' > "$meta"
first=1
count=0

for src in "$ds"/src/cards/*.html; do
  [ -e "$src" ] || { echo "нет ни одного исходника в src/cards" >&2; exit 1; }
  name="$(basename "$src" .html)"
  tr -d '\r' < "$src" > "$tmp"

  group="$(directive dsCard)"
  label="$(directive dsName)"
  subtitle="$(directive dsSubtitle)"
  csslist="$(directive dsCss)"
  viewport="$(directive dsViewport)"
  theme="$(directive dsTheme)"

  [ -n "$group" ] || { echo "$name: нет директивы @dsCard group=\"...\"" >&2; exit 1; }
  [ -n "$label" ] || label="$name"
  groupval="$(printf %s "$group" | sed 's/.*group="\([^"]*\)".*/\1/')"
  [ -n "$viewport" ] || viewport="900x560"
  vw="${viewport%%x*}"
  vh="${viewport##*x}"
  body_class="ds-page"
  if [ "$theme" = "dark" ]; then body_class="ds-page ds-page--dark"; fi

  dst="$out/components/$name.html"
  {
    printf '<!-- @dsCard %s -->\n' "$group"
    printf '<!DOCTYPE html>\n<html lang="ru">\n<head>\n<meta charset="utf-8">\n'
    printf '<meta name="viewport" content="width=device-width, initial-scale=1">\n'
    printf '<title>%s</title>\n<style>\n' "$label"
    for f in $csslist; do
      [ -f "$static/$f" ] || { echo "$name: нет $static/$f" >&2; exit 1; }
      printf '/* --- web/static/%s --- */\n' "$f"
      tr -d '\r' < "$static/$f"
      printf '\n'
    done
    printf '/* --- design-system/src/preview.css (обвязка превью, не часть продукта) --- */\n'
    tr -d '\r' < "$ds/src/preview.css"
    printf '\n</style>\n</head>\n<body class="%s">\n<div class="ds-wrap">\n' "$body_class"
    grep -v '^<!-- @ds' "$tmp"
    printf '</div>\n</body>\n</html>\n'
  } > "$dst"

  [ $first -eq 1 ] || printf ',\n' >> "$meta"
  first=0
  printf '  {"path": "components/%s.html", "name": "%s", "group": "%s", "subtitle": "%s", "viewport": {"width": %s, "height": %s}}' \
    "$name" "$(json_escape "$label")" "$(json_escape "$groupval")" "$(json_escape "$subtitle")" "$vw" "$vh" >> "$meta"
  count=$((count + 1))
done

printf '\n]\n' >> "$meta"
echo "собрано карточек: $count -> $out/components"
