import http from "node:http";

const port = 4175;

http
  .createServer(async (request, response) => {
    if (request.url === "/healthz") {
      response.writeHead(200).end("ok");
      return;
    }

    let rawBody = "";
    for await (const chunk of request) {
      rawBody += chunk;
    }
    const body = JSON.parse(rawBody);
    const userMessages = body.messages.filter((message) => message.role === "user");
    const latest = userMessages.at(-1)?.content ?? "";

    if (userMessages.some((message) => message.content.includes("浏览器失败"))) {
      response.writeHead(503, { "Content-Type": "application/json" });
      response.end('{"error":{"message":"scripted outage"}}');
      return;
    }

    let interview;
    if (latest.includes("继续完成恢复测试")) {
      interview = {
        reply: "进度恢复成功，已经建立 Candidate pool。",
        complete: true,
        preferences: [{ dish_name: "番茄牛腩", weight: 4.5 }],
      };
    } else if (latest.includes("浏览器恢复")) {
      interview = {
        reply: "番茄牛腩记下了，再确认一下它有多喜欢？",
        complete: false,
        preferences: [],
      };
    } else {
      interview = {
        reply: "已经按你的偏好建立 Candidate pool。",
        complete: true,
        preferences: [{ dish_name: "番茄炒蛋", weight: 5 }],
      };
    }

    response.writeHead(200, { "Content-Type": "application/json" });
    response.end(
      JSON.stringify({
        choices: [{ message: { content: JSON.stringify(interview) } }],
      }),
    );
  })
  .listen(port, "127.0.0.1");
