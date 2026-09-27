import * as React from "react"
import { cn } from "cn"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "ui-focus ui-field flex field-sizing-content min-h-16 w-full px-3 py-2",
        className
      )}
      {...props}
    />
  )
}

export { Textarea }
