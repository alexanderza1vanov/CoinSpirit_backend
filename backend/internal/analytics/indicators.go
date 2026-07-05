package analytics

import "math"

func SMA(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	for i := period - 1; i < len(values); i++ {
		sum := 0.0
		for j := i - period + 1; j <= i; j++ {
			sum += values[j]
		}
		result = append(result, sum/float64(period))
	}
	return result
}

func EMA(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	prev := sum / float64(period)
	result = append(result, prev)
	multiplier := 2.0 / float64(period+1)
	for i := period; i < len(values); i++ {
		current := values[i]*multiplier + prev*(1-multiplier)
		result = append(result, current)
		prev = current
	}
	return result
}

func RSI(values []float64, period int) []float64 {
	if period <= 0 || len(values) <= period {
		return nil
	}
	gain := 0.0
	loss := 0.0
	for i := 1; i <= period; i++ {
		change := values[i] - values[i-1]
		if change >= 0 {
			gain += change
		} else {
			loss -= change
		}
	}
	avgGain := gain / float64(period)
	avgLoss := loss / float64(period)
	result := make([]float64, 0, len(values)-period)
	for i := period + 1; i < len(values); i++ {
		change := values[i] - values[i-1]
		currentGain := 0.0
		currentLoss := 0.0
		if change >= 0 {
			currentGain = change
		} else {
			currentLoss = -change
		}
		avgGain = (avgGain*float64(period-1) + currentGain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + currentLoss) / float64(period)
		if avgLoss == 0 {
			result = append(result, 100)
			continue
		}
		rs := avgGain / avgLoss
		result = append(result, 100-100/(1+rs))
	}
	return result
}

type MACDPoint struct {
	MACD      float64
	Signal    float64
	Histogram float64
}

func MACD(values []float64) []MACDPoint {
	ema12 := EMA(values, 12)
	ema26 := EMA(values, 26)
	if len(ema12) == 0 || len(ema26) == 0 {
		return nil
	}
	offset := len(ema12) - len(ema26)
	macdLine := make([]float64, 0, len(ema26))
	for i := 0; i < len(ema26); i++ {
		macdLine = append(macdLine, ema12[i+offset]-ema26[i])
	}
	signal := EMA(macdLine, 9)
	if len(signal) == 0 {
		return nil
	}
	signalOffset := len(macdLine) - len(signal)
	result := make([]MACDPoint, 0, len(signal))
	for i := 0; i < len(signal); i++ {
		macd := macdLine[i+signalOffset]
		result = append(result, MACDPoint{MACD: macd, Signal: signal[i], Histogram: macd - signal[i]})
	}
	return result
}

type BollingerBandPoint struct {
	Middle float64
	Upper  float64
	Lower  float64
}

func BollingerBands(values []float64, period int, multiplier float64) []BollingerBandPoint {
	if period <= 0 || len(values) < period {
		return nil
	}
	result := make([]BollingerBandPoint, 0, len(values)-period+1)
	for i := period - 1; i < len(values); i++ {
		window := values[i-period+1 : i+1]
		sum := 0.0
		for _, v := range window {
			sum += v
		}
		mean := sum / float64(period)
		variance := 0.0
		for _, v := range window {
			diff := v - mean
			variance += diff * diff
		}
		stdDev := math.Sqrt(variance / float64(period))
		result = append(result, BollingerBandPoint{Middle: mean, Upper: mean + multiplier*stdDev, Lower: mean - multiplier*stdDev})
	}
	return result
}

func Last(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return values[len(values)-1]
}
