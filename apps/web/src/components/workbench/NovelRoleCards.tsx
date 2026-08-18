"use client";

import { useMemo } from "react";

type Role = {
  id: string;
  name: string;
  avatar: string;
  description: string;
  node: string;
};

type NovelRoleCardsProps = {
  roles: Role[];
  currentNode?: string;
  className?: string;
};

export function NovelRoleCards({ roles, currentNode, className = "" }: NovelRoleCardsProps) {
  const displayRoles = useMemo(() => {
    if (!roles || roles.length === 0) {
      return DEFAULT_ROLES;
    }
    return roles;
  }, [roles]);

  return (
    <div className={`novel-role-cards mt-5 ${className}`}>
      <div className="flex items-center justify-center gap-3 overflow-x-auto pb-1">
        {displayRoles.map((role) => {
          const isActive = role.node === currentNode;
          return (
            <div
              key={role.id}
              className={`
                relative flex min-w-[72px] flex-col items-center gap-1.5 rounded-xl px-2 py-2 transition-all duration-300
                ${
                  isActive ? "bg-white/[0.07]" : "hover:bg-white/[0.04]"
                }
              `}
              title={role.description}
            >
              {/* 角色头像 */}
              <div
                className={`
                  flex h-11 w-11 items-center justify-center rounded-full border border-indigo-300/30 bg-indigo-500/10 text-2xl shadow-[0_0_18px_rgba(99,102,241,.14)] transition-transform duration-300
                  ${isActive ? "scale-105" : ""}
                `}
              >
                {role.avatar}
              </div>

              {/* 角色名称 */}
              <div
                className={`
                  truncate text-[11px] font-medium text-center transition-colors
                  ${isActive ? "text-indigo-200" : "text-gray-400"}
                `}
              >
                {role.name}
              </div>

              {/* 工作中指示器 */}
              {isActive && (
                <div className="absolute -top-1 -right-1">
                    <span className="relative flex h-2.5 w-2.5">
                    <span className="absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-50"></span>
                    <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-400"></span>
                  </span>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// 默认角色配置（作为降级方案）
const DEFAULT_ROLES: Role[] = [
  {
    id: "chief_editor",
    name: "总编主管",
    avatar: "👔",
    description: "统筹整本书的创作，调度团队、把控节奏与整体质量",
    node: "planning",
  },
  {
    id: "story_planner",
    name: "故事策划",
    avatar: "📋",
    description: "出故事方向、搭故事圣经、排卷章大纲",
    node: "planning",
  },
  {
    id: "rhythm_editor",
    name: "节奏编排师",
    avatar: "📊",
    description: "排张力曲线：钩子、爆点、悬念落位",
    node: "planning",
  },
  {
    id: "chapter_writer",
    name: "章节写手",
    avatar: "✍️",
    description: "按大纲和设定档案逐章写正文",
    node: "writing",
  },
  {
    id: "polish_writer",
    name: "文学润色师",
    avatar: "✨",
    description: "只改语言不改情节，让文字更有质感",
    node: "polishing",
  },
  {
    id: "proofreader",
    name: "审校员",
    avatar: "✅",
    description: "对照设定台账查矛盾：时间线、人物状态",
    node: "polishing",
  },
  {
    id: "archivist",
    name: "档案员",
    avatar: "📁",
    description: "更新人物状态与摘要，设定不崩",
    node: "archiving",
  },
];
