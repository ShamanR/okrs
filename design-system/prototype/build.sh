#!/usr/bin/env bash
# Сборка кликабельного прототипа в design-system/dist/prototype.html.
#
# Тот же принцип, что и у карточек дизайн-системы: продуктовый CSS не дублируется,
# а инлайнится с диска. Прототип поэтому выглядит ровно так же, как приложение,
# и не расходится с ним при правках стилей.
#
# Своё здесь только три вещи: overlay.css (обвязка прототипа), data.js (тестовые
# данные) и app.js (рендер и поведение).
set -euo pipefail

proto="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ds="$(dirname "$proto")"
root="$(dirname "$ds")"
static="$root/web/static"
out="$ds/dist/prototype.html"
local_out="$ds/dist/prototype.local.html"

mkdir -p "$ds/dist"

# Порядок важен: tokens первым, дальше каркас, дальше компоненты — как в shell-шаблонах.
css_files="tokens.css shell.css components.css sidebar.css markdown.css tracker.css goal_tree.css activity.css"

emit_styles() {
  printf '<title>Прототип трекера OKR</title>\n<style>\n'
  for f in $css_files; do
    [ -f "$static/$f" ] || { echo "нет $static/$f" >&2; exit 1; }
    printf '/* --- web/static/%s --- */\n' "$f"
    tr -d '\r' < "$static/$f"
    printf '\n'
  done
  printf '/* --- design-system/prototype/overlay.css --- */\n'
  tr -d '\r' < "$proto/overlay.css"
  printf '\n</style>\n'
}

emit_body() {
  tr -d '\r' < "$proto/body.html"
  printf '\n<script>\n'
  tr -d '\r' < "$proto/data.js"
  printf '\n'
  tr -d '\r' < "$proto/app.js"
  printf '\n</script>\n'
}

# 1. Версия для публикации: артефактный движок сам оборачивает её в <!DOCTYPE>,
#    <html>, <head> и <body>, поэтому здесь их быть не должно. Маркер @dsCard
#    первой строкой нужен панели Design System.
{
  printf '<!-- @dsCard group="Прототип" -->\n'
  emit_styles
  printf '\n'
  emit_body
} > "$out"

# 2. Версия для локального просмотра: полноценный документ, открывается файлом
#    в браузере без сервера — всё инлайн, внешних запросов нет.
{
  printf '<!DOCTYPE html>\n<html lang="ru">\n<head>\n<meta charset="utf-8">\n'
  printf '<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">\n'
  emit_styles
  printf '</head>\n<body>\n'
  emit_body
  printf '</body>\n</html>\n'
} > "$local_out"

echo "собран $out ($(wc -c < "$out" | tr -d ' ') байт) — для публикации"
echo "собран $local_out ($(wc -c < "$local_out" | tr -d ' ') байт) — для локального просмотра"
