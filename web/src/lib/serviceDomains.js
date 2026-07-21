const serviceName = service => String(service?.name || '').toLowerCase()

const isReflectService = service => service?.kind === 'reflect' || serviceName(service).startsWith('reflect/')

export const scheduleServices = services => (Array.isArray(services) ? services : [])
  .filter(service => serviceName(service).includes('scheduler'))

export const autonomousServices = services => (Array.isArray(services) ? services : [])
  .filter(service => {
    const name = serviceName(service)
    return isReflectService(service) && !name.includes('scheduler') && !name.includes('goal_mode')
  })
