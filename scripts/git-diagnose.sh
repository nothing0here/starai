#!/usr/bin/env bash
# 生产环境小说工作流代码同步诊断脚本
# 用法：cd /www/wwwroot/starai && bash git-diagnose.sh
set +e
echo "===== 1. 远程地址 ====="
git remote -v

echo ""
echo "===== 2. fetch 是否成功（重点看有无报错） ====="
git fetch origin -v

echo ""
echo "===== 3. 远程 main 最新提交（应包含 48c400d 新增AI小说工坊） ====="
git log --oneline -5 origin/main

echo ""
echo "===== 4. 本地 HEAD 位置 ====="
git log --oneline -3
git status -sb | head -3

echo ""
echo "===== 5. 小说工作流关键文件是否存在 ====="
for f in \
  services/worker/cmd/worker/novel_workflow.go \
  apps/web/src/components/workbench/NovelWorkshopLanding.tsx \
  apps/web/src/components/workbench/NovelPlayGuide.tsx; do
  if [ -f "$f" ]; then echo "存在: $f"; else echo "缺失: $f"; fi
done
