package csp

// IsPrime returns true if n is a prime number (n >= 2).
func IsPrime(n int) bool {
	if n < 2 {
		return false
	}
	if n < 4 {
		return true
	}
	if n%2 == 0 || n%3 == 0 {
		return false
	}
	for i := 5; i*i <= n; i += 6 {
		if n%i == 0 || n%(i+2) == 0 {
			return false
		}
	}
	return true
}

// NthPrime returns the n-th prime (0-indexed: NthPrime(0)=2, NthPrime(1)=3, ...).
// For negative n, returns 2.
func NthPrime(n int) int {
	if n <= 0 {
		return 2
	}
	count := 0
	candidate := 2
	for {
		if IsPrime(candidate) {
			if count == n {
				return candidate
			}
			count++
		}
		candidate++
	}
}

// GCD returns the greatest common divisor of |a| and |b| using Euclid's algorithm.
// GCD(0, 0) = 0.
func GCD(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// Fibonacci returns the n-th Fibonacci number (0-indexed: F(0)=0, F(1)=1, F(2)=1, ...).
// For negative n, returns 0.
func Fibonacci(n int) int {
	if n <= 0 {
		return 0
	}
	if n == 1 {
		return 1
	}
	a, b := 0, 1
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}

// DigitRoot computes the digital root of n (repeated digit sum until single digit).
// For n <= 0, returns 0.
func DigitRoot(n int) int {
	if n < 0 {
		n = -n
	}
	if n == 0 {
		return 0
	}
	return 1 + (n-1)%9
}
