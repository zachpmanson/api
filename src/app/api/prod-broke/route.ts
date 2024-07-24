import { type NextRequest } from "next/server";
import { z } from "zod";
import prisma from "../../../server/prisma";
import { DEPLOYMENT_ENVIRONMENTS } from "@/types/prod-broke";

export const revalidate = 0;
export async function GET(request: NextRequest) {
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

  return Response.json(latestBrokenDeployments);
}

const schema = z.object({
  deployment: z.enum(DEPLOYMENT_ENVIRONMENTS),
  date: z.string().pipe(z.coerce.date()),
  key: z.string(),
});

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
