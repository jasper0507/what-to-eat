import { Link } from "react-router-dom";

import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export default function NotFoundPage() {
  return (
    <div className="animate-rise flex flex-col items-center gap-6 py-16 text-center">
      <span
        aria-hidden="true"
        className="font-serif text-6xl leading-none text-brand"
      >
        ？
      </span>
      <div className="space-y-1.5">
        <h1 className="font-serif text-2xl font-medium">这里什么都没有。</h1>
        <p className="text-sm text-muted-foreground">
          地址不对。回主页，接着定这一顿。
        </p>
      </div>
      <Link to="/" className={cn(buttonVariants({ size: "lg" }))}>
        回主页
      </Link>
    </div>
  );
}
