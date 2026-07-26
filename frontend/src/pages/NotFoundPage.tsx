import { Button } from "antd";
import { CookingPot } from "lucide-react";
import { m } from "motion/react";

import { copy } from "@/lib/copy";
import { pageEnter } from "@/lib/motion";

export default function NotFoundPage() {
  return (
    <m.div {...pageEnter} className="container page-stack notfound">
      <CookingPot
        size={56}
        strokeWidth={1.5}
        className="notfound-icon"
        aria-hidden="true"
      />
      <h1 className="page-title">{copy.notFound.title}</h1>
      <p className="page-intro">{copy.notFound.intro}</p>
      <div>
        <Button type="primary" size="large" href="/">
          {copy.notFound.home}
        </Button>
      </div>
    </m.div>
  );
}
