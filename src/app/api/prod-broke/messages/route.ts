import prisma from "@/server/prisma";
import { DEPLOYMENT_ENVIRONMENTS } from "@/types/prod-broke";

export const revalidate = 0;

export async function GET() {
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
