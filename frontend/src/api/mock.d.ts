export interface MockApiResponse {
  data: any
  status: number
  headers: Headers
}

export function mockRequest(method: string, url: string, data?: any): Promise<MockApiResponse>
export function mockFetchExport(includeSubscriptions?: boolean): Promise<Response>
