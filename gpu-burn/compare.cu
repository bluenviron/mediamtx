/* 
 * Copyright (c) 2016, Ville Timonen
 * All rights reserved.
 * 
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are met:
 * 
 * 1. Redistributions of source code must retain the above copyright notice, this
 *    list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright notice,
 *    this list of conditions and the following disclaimer in the documentation
 *    and/or other materials provided with the distribution.
 * 
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
 * ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 * WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
 * DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
 * ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
 * (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
 * LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
 * ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
 * SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 * 
 * The views and conclusions contained in the software and documentation are those
 * of the authors and should not be interpreted as representing official policies,
 * either expressed or implied, of the FreeBSD Project.
 */

// Actually, there are no rounding errors due to results being accumulated in an arbitrary order..
// Therefore EPSILON = 0.0f is OK
#define EPSILON 0.001f
#define EPSILOND 0.0000001

extern "C" __global__ void compare(float *C, int *faultyElems, size_t iters) {
	size_t iterStep = blockDim.x*blockDim.y*gridDim.x*gridDim.y;
	size_t myIndex = (blockIdx.y*blockDim.y + threadIdx.y)* // Y
		gridDim.x*blockDim.x + // W
		blockIdx.x*blockDim.x + threadIdx.x; // X

	int myFaulty = 0;
	for (size_t i = 1; i < iters; ++i)
		if (fabsf(C[myIndex] - C[myIndex + i*iterStep]) > EPSILON)
			myFaulty++;

	atomicAdd(faultyElems, myFaulty);
}

extern "C" __global__ void compareD(double *C, int *faultyElems, size_t iters) {
	size_t iterStep = blockDim.x*blockDim.y*gridDim.x*gridDim.y;
	size_t myIndex = (blockIdx.y*blockDim.y + threadIdx.y)* // Y
		gridDim.x*blockDim.x + // W
		blockIdx.x*blockDim.x + threadIdx.x; // X

	int myFaulty = 0;
	for (size_t i = 1; i < iters; ++i)
		if (fabs(C[myIndex] - C[myIndex + i*iterStep]) > EPSILOND)
			myFaulty++;

	atomicAdd(faultyElems, myFaulty);
}
