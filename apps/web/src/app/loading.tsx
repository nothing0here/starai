"use client";

import { useI18n } from "@/i18n/I18nProvider";

export default function Loading() {
  const { ts } = useI18n();
  return <div className="flex min-h-screen items-center justify-center bg-gray-50 text-sm text-gray-400 dark:bg-gray-950"><span className="mr-3 h-5 w-5 animate-spin rounded-full border-2 border-gray-200 border-t-gray-800 dark:border-white/20 dark:border-t-white" />{ts("页面加载中...")}</div>;
}
