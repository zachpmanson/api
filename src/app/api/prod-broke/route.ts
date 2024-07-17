import { Prisma } from "@prisma/client";
import { NextResponse, type NextRequest } from "next/server";
import prisma from "../../../server/prisma";
import { date, z } from "zod";

export async function GET(request: NextRequest, context: { params: Params }) {
  const deployments = await prisma.brokenDeployment.groupBy({
    by: ["deployment"],
    _max: {
      date: true,
    },
  });

  const latestBrokenDeployments = await Promise.all(
    deployments.map(async (deployment) => {
      return await prisma.brokenDeployment.findFirst({
        where: {
          deployment: deployment.deployment,
          date: deployment._max.date ?? undefined,
        },
      });
    })
  );

  if (request.nextUrl.searchParams.get("pretty") === "true") {
    let res: Record<string, any> = {};
    for (let d of DEPLOYMENT_ENVIRONMENTS) {
      const brokenDeployment = latestBrokenDeployments.find(
        (brokenDeployment) =>
          !!brokenDeployment && brokenDeployment.deployment === d
      );
      if (brokenDeployment) {
        const diff = Date.now() - new Date(brokenDeployment.date).getTime();
        const days = Math.floor(diff / (1000 * 60 * 60 * 24));
        res[d] = `${days.toString()} days since last broken ${d} deployment`;
      } else {
        res[d] = `${d} has never had a broken deployment`;
      }
    }
    return Response.json(res);
  }

  return Response.json(latestBrokenDeployments);
}

const DEPLOYMENT_ENVIRONMENTS: readonly ["PROD", "STG"] = ["PROD", "STG"];
const schema = z.object({
  deployment: z.enum(DEPLOYMENT_ENVIRONMENTS),
  date: z.string().pipe(z.coerce.date()),
  key: z.string(),
});

type Params = z.infer<typeof schema>;

export async function POST(req: NextRequest) {
  const response = schema.safeParse(await req.json());
  if (!response.success) {
    return new Response(
      JSON.stringify({ status: "Malformed", error: response.error }),
      {
        status: 400,
      }
    );
  }

  if (response.data.key !== process.env.PROD_BROKE_KEY) {
    return new Response("Password incorrect", { status: 401 });
  }

  const brokenDeployment = await prisma.brokenDeployment.create({
    data: {
      deployment: response.data.deployment,
      date: response.data.date,
    },
  });
  return Response.json({ success: true, data: brokenDeployment });
}
