"use client"

import { useTheme } from "next-themes"
import { Toaster as Sonner, type ToasterProps } from "sonner"
import {
  CircleCheckIcon,
  InfoIcon,
  TriangleAlertIcon,
  OctagonXIcon,
  Loader2Icon,
} from "lucide-react"

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      className="toaster group"
      closeButton
      visibleToasts={3}
      duration={4_500}
      containerAriaLabel="操作提示"
      icons={{
        success: <CircleCheckIcon className="size-4" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 motion-safe:animate-spin" />,
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
        } as React.CSSProperties
      }
      toastOptions={{
        closeButtonAriaLabel: "关闭提示",
        classNames: {
          toast:
            "cn-toast pointer-events-none max-w-[calc(100vw-2rem)] transition-[opacity,transform,background-color,border-color] duration-200",
          title: "font-medium",
          description:
            "line-clamp-2 text-muted-foreground sm:line-clamp-none",
          actionButton:
            "pointer-events-auto bg-primary text-primary-foreground hover:bg-primary/90",
          cancelButton:
            "pointer-events-auto bg-muted text-muted-foreground hover:bg-muted/80",
          closeButton:
            "pointer-events-auto border-border bg-background text-muted-foreground hover:text-foreground",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
