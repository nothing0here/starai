"use client";

import { useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Check, ChevronDown } from "lucide-react";

export function MediaOptionMenu({
  icon,
  label,
  activeLabel,
  title,
  subtitle,
  tone = "white",
  compactOnMobile = false,
  children,
}: {
  icon: ReactNode;
  label?: string;
  activeLabel: string;
  title: string;
  subtitle: string;
  tone?: "white" | "yellow";
  compactOnMobile?: boolean;
  children: (close: () => void) => ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const btnRef = useRef<HTMLButtonElement | null>(null);
  const [pos, setPos] = useState({ left: 0, bottom: 0 });

  const openMenu = () => {
    const rect = btnRef.current?.getBoundingClientRect();
    if (rect) {
      const menuWidth = 220;
      const viewportGap = 8;
      setPos({
        left: Math.max(viewportGap, Math.min(rect.left, window.innerWidth - menuWidth - viewportGap)),
        bottom: Math.max(viewportGap, window.innerHeight - rect.top + viewportGap),
      });
    }
    setOpen((v) => !v);
  };

  return (
    <>
      <button
        ref={btnRef}
        type="button"
        onClick={openMenu}
        className={`h-8 rounded-xl border text-xs flex items-center gap-1.5 shadow-sm transition ${compactOnMobile ? "px-2 sm:px-2.5" : "px-2.5"} ${
          tone === "yellow"
            ? "bg-amber-100 border-amber-300 text-gray-900 dark:bg-amber-500/10 dark:border-amber-400/30 dark:text-amber-100"
            : "bg-white border-gray-200 text-gray-700 dark:bg-white/5 dark:border-white/10 dark:text-gray-200"
        }`}
      >
        <span className="text-gray-500 dark:text-gray-400">{icon}</span>
        <span className={compactOnMobile ? "hidden sm:inline" : ""}>{activeLabel || label}</span>
        <ChevronDown size={13} className={`text-gray-500 transition dark:text-gray-400 ${open ? "rotate-180" : ""}`} />
      </button>
      {open && typeof document !== "undefined"
        ? createPortal(
            <div className="fixed inset-0 z-[70]" onClick={() => setOpen(false)}>
              <div
                className="fixed w-[220px] max-w-[calc(100vw-1rem)] overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-white/10 dark:bg-gray-900"
                style={{ left: pos.left, bottom: pos.bottom }}
                onClick={(e) => e.stopPropagation()}
              >
                <div className="border-b border-gray-100 px-4 py-2.5 dark:border-white/10">
                  <div className="text-sm font-bold text-gray-900 dark:text-gray-100">{title}</div>
                  <div className="mt-1 text-[11px] leading-relaxed text-gray-500 dark:text-gray-400">{subtitle}</div>
                </div>
                <div className="max-h-[280px] overflow-y-auto bg-white p-2.5 dark:bg-gray-900">{children(() => setOpen(false))}</div>
              </div>
            </div>,
            document.body,
          )
        : null}
    </>
  );
}

export function MediaMenuOption({
  selected,
  children,
  onClick,
}: {
  selected: boolean;
  children: ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`w-full h-10 px-3.5 rounded-xl text-left flex items-center justify-between text-sm font-semibold ${
        selected
          ? "bg-primary/10 dark:bg-primary/15 text-gray-900 dark:text-gray-100"
          : "bg-gray-50 hover:bg-gray-100 dark:bg-white/5 dark:text-gray-300 dark:hover:bg-white/10"
      }`}
    >
      <span>{children}</span>
      {selected && <Check size={16} className="text-primary" />}
    </button>
  );
}
